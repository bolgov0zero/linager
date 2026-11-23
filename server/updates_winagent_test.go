package server

import "testing"

func TestWindowsByHand(t *testing.T) {
	cases := []struct {
		os, ver string
		has     bool
		want    bool
	}{
		{"Windows Server 2022 Standard (21H2, build 20348)", "v0.3.0-beta.3", true, true}, // before self-update: a new MSI
		{"Windows 11 Pro", "v0.3.0-beta.4", true, false},                                  // updates from the panel
		{"Windows 11 Pro", "v0.3.1", true, false},                                         // newer than the first one
		{"Windows 11 Pro", "v0.3.0-beta.4", false, true},                                  // the release has no Windows file
		{"Debian GNU/Linux 12 (bookworm)", "v0.2.9", true, false},                         // Linux is never by hand
		{"Debian GNU/Linux 12 (bookworm)", "v0.3.0-beta.1", false, false},
	}
	for _, c := range cases {
		if got := windowsByHand(c.os, c.ver, c.has); got != c.want {
			t.Errorf("%q %s has=%v: got %v, want %v", c.os, c.ver, c.has, got, c.want)
		}
	}
}

func TestWinActionsUseRealOperatorRights(t *testing.T) {
	for key, act := range winActions {
		if !validModule(act.right) {
			t.Errorf("%s needs the right %q, which an operator cannot be given", key, act.right)
		}
		if act.timeout <= 0 {
			t.Errorf("%s has no timeout", key)
		}
	}
	// reading is never dangerous, changing always is, except the two calls that settle a network change
	for _, read := range []string{"users.state", "winupdate.list", "network.state", "apps.list"} {
		if winActions[read].dangerous {
			t.Errorf("%s only reads", read)
		}
	}
	for _, change := range []string{"users.add", "users.update", "users.passwd", "users.delete", "users.logoff", "network.apply"} {
		if !winActions[change].dangerous {
			t.Errorf("%s changes the computer and needs the confirmation", change)
		}
	}
	if !winUpdateIDRe.MatchString("0f3a2b1c-1234-5678-9abc-def012345678") || winUpdateIDRe.MatchString("x; rm") {
		t.Error("update ids are GUIDs")
	}
}
