package server_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

type devJSON struct {
	ID         int64  `json:"id"`
	User       string `json:"user"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	OSVersion  string `json:"os_version"`
	Model      string `json:"model"`
	Status     string `json:"status"`
	LastIP     string `json:"last_ip"`
	Current    bool   `json:"current"`
}

type devState struct {
	Enabled   bool      `json:"enabled"`
	Pending   int       `json:"pending"`
	CanEnable bool      `json:"can_enable"`
	Devices   []devJSON `json:"devices"`
}

// deviceClient is a client of an application: it has the token of its device and says what it is.
func deviceClient(t *testing.T, base, token, platform, model string) *apiClient {
	jar, _ := cookiejar.New(nil)
	return &apiClient{t: t, base: base, hc: testClient(jar), headers: map[string]string{
		"X-Linager-Device": token,
		"X-Linager-Client": "platform=" + platform + "&app=1.2.3&os=27.1&model=" + model,
	}}
}

func token(c byte) string { return strings.Repeat(string(c), 43) }

func devices(t *testing.T, c *apiClient) devState {
	t.Helper()
	var st devState
	if code := c.do("GET", "/api/devices", nil, &st); code != 200 {
		t.Fatalf("devices: %d", code)
	}
	return st
}

// Devices work end to end: they are listed while the confirmation is off; once it is on a new device waits and may only ask who it is
// and sign out; an administrator approves or refuses it; removing a device signs it out; the command line of the server turns the
// confirmation off again.
func TestDevicesConfirmation(t *testing.T) {
	base, db := startAccountsServer(t)
	ctx := context.Background()

	// the desktop application of the administrator: its device is listed and approved while nothing is confirmed
	desk := deviceClient(t, base, token('D'), "macos", "Mac")
	if code, out := desk.login("admin", "a-long-test-password", nil); code != 200 {
		t.Fatalf("login: %d", code)
	} else if dv, _ := out["device"].(map[string]any); dv["status"] != "approved" || dv["confirm"] != false {
		t.Fatalf("device of the sign-in: %v", out["device"])
	}
	st := devices(t, desk)
	if st.Enabled || len(st.Devices) != 1 || st.Devices[0].Status != "approved" || !st.Devices[0].Current || !st.CanEnable ||
		st.Devices[0].Platform != "macos" || st.Devices[0].AppVersion != "1.2.3" || st.Devices[0].Model != "Mac" || st.Devices[0].LastIP == "" {
		t.Fatalf("the first device: %+v", st)
	}

	// a client that has no device cannot turn the confirmation on: the administrator would be locked out
	plain := mustLogin(t, base, "admin", "a-long-test-password")
	if code := plain.do("PUT", "/api/devices/gate", map[string]any{"enabled": true, "confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("no device, no gate: %d", code)
	}
	if code := desk.do("PUT", "/api/devices/gate", map[string]any{"enabled": true}, nil); code != http.StatusConflict {
		t.Fatalf("the gate is confirmed: %d", code)
	}
	if code := desk.do("PUT", "/api/devices/gate", map[string]any{"enabled": true, "keep_sessions": true, "confirm": true}, nil); code != 200 {
		t.Fatalf("gate on: %d", code)
	}
	if !devices(t, desk).Enabled {
		t.Fatal("the confirmation is on")
	}
	// the session that has no device (an old client) is cut off with the code that tells it to update; the desktop goes on
	if code := plain.do("GET", "/api/dashboard", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a client without a device: %d", code)
	}
	if code := desk.do("GET", "/api/dashboard", nil, nil); code != 200 {
		t.Fatalf("the approved device goes on: %d", code)
	}

	// a phone signs in: the password is right, the session opens, nothing else is allowed
	phone := deviceClient(t, base, token('P'), "ios", "iPhone18,1")
	code, out := phone.login("admin", "a-long-test-password", nil)
	if code != 200 {
		t.Fatalf("login of the phone: %d", code)
	}
	if dv, _ := out["device"].(map[string]any); dv["status"] != "pending" || dv["confirm"] != true {
		t.Fatalf("the phone waits: %v", out["device"])
	}
	if code := phone.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatalf("a waiting device may ask who it is: %d", code)
	}
	for _, p := range []string{"/api/dashboard", "/api/devices", "/api/live", "/api/groups"} {
		var e struct{ Error string }
		if code := phone.do("GET", p, nil, &e); code != http.StatusForbidden || e.Error != server.ErrApprovalPending {
			t.Fatalf("%s while waiting: %d %q", p, code, e.Error)
		}
	}
	st = devices(t, desk)
	if st.Pending != 1 || len(st.Devices) != 2 || st.Devices[0].Status != "pending" || st.Devices[0].Platform != "ios" || st.Devices[0].Current {
		t.Fatalf("the waiting phone is first: %+v", st)
	}
	phoneID := st.Devices[0].ID

	// the phone signs in again with the same device: the same device, no second request
	phone2 := deviceClient(t, base, token('P'), "ios", "iPhone18,1")
	if code, _ := phone2.login("admin", "a-long-test-password", nil); code != 200 {
		t.Fatalf("second sign-in: %d", code)
	}
	if st = devices(t, desk); len(st.Devices) != 2 {
		t.Fatalf("one device, not two: %+v", st)
	}
	// a client that says nothing of a device cannot sign in
	if code, out := newClient(t, base).login("admin", "a-long-test-password", nil); code != http.StatusForbidden || out["error"] != server.ErrDeviceRequired {
		t.Fatalf("sign-in with no device: %d %v", code, out)
	}
	// a stolen cookie is no good without the token of the device
	thief := deviceClient(t, base, token('X'), "ios", "iPhone")
	thief.hc = phone.hc
	if code := thief.do("GET", "/api/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("a cookie from another device: %d", code)
	}

	// approve: the phone is in
	if code := desk.do("POST", "/api/devices/"+itoa(phoneID)+"/approve", nil, nil); code != 200 {
		t.Fatalf("approve: %d", code)
	}
	if code := phone.do("GET", "/api/dashboard", nil, nil); code != 200 {
		t.Fatalf("approved phone: %d", code)
	}
	// ... and an operator cannot approve anything
	if code := desk.do("POST", "/api/users", map[string]any{"login": "oper", "password": "operator-long-password", "role": "operator"}, nil); code != 200 {
		t.Fatalf("create operator: %d", code)
	}
	opPhone := deviceClient(t, base, token('O'), "ios", "iPhone")
	if code, _ := opPhone.login("oper", "operator-long-password", nil); code != 200 {
		t.Fatalf("operator login: %d", code)
	}
	if code := opPhone.do("POST", "/api/devices/"+itoa(phoneID)+"/approve", nil, nil); code != http.StatusForbidden {
		t.Fatalf("an operator does not approve: %d", code)
	}

	// deny: the device is refused at once, its sessions end, and it cannot sign in again
	var opID int64
	for _, d := range devices(t, desk).Devices {
		if d.User == "oper" {
			opID = d.ID
		}
	}
	if code := desk.do("POST", "/api/devices/"+itoa(opID)+"/deny", nil, nil); code != 200 {
		t.Fatalf("deny: %d", code)
	}
	if code := opPhone.do("GET", "/api/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("the session of a refused device is gone: %d", code)
	}
	if code, out := deviceClient(t, base, token('O'), "ios", "iPhone").login("oper", "operator-long-password", nil); code != http.StatusForbidden || out["error"] != server.ErrDeviceDenied {
		t.Fatalf("a refused device cannot sign in: %d %v", code, out)
	}
	// the administrator cannot refuse the device they are using
	if code := desk.do("POST", "/api/devices/"+itoa(devices(t, desk).Devices[currentIndex(devices(t, desk))].ID)+"/deny", nil, nil); code != http.StatusConflict {
		t.Fatalf("not the own device: %d", code)
	}

	// remove needs the confirmation; a removed device is signed out
	if code := desk.do("DELETE", "/api/devices/"+itoa(phoneID), nil, nil); code != http.StatusConflict {
		t.Fatalf("remove is confirmed: %d", code)
	}
	if code := desk.do("DELETE", "/api/devices/"+itoa(phoneID), map[string]any{"confirm": true}, nil); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	if code := phone.do("GET", "/api/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("a removed device is signed out: %d", code)
	}

	// a name for a device
	if code := desk.do("PUT", "/api/devices/"+itoa(devices(t, desk).Devices[currentIndex(devices(t, desk))].ID), map[string]string{"label": "My Mac"}, nil); code != 200 {
		t.Fatalf("label: %d", code)
	}

	// the audit has the story
	for _, action := range []string{"device.gate.on", "device.request", "device.approve", "device.deny", "device.remove", "device.label"} {
		var n int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action=$1`, action).Scan(&n); err != nil || n == 0 {
			t.Errorf("audit %s: %d %v", action, n, err)
		}
	}

	// the way back: the command line turns the confirmation off and approves a device, with no restart
	if err := server.DevicesGateOff(ctx, db); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the gate is off", func() bool { return !devices(t, desk).Enabled })
	if code := plain.do("GET", "/api/dashboard", nil, nil); code != 200 {
		t.Fatalf("with the confirmation off the client with no device is in: %d", code)
	}
	var sb strings.Builder
	if err := server.DevicesList(ctx, db, &sb); err != nil || !strings.Contains(sb.String(), "off") || !strings.Contains(sb.String(), "My Mac") {
		t.Fatalf("list: %v\n%s", err, sb.String())
	}
	if err := server.DeviceApproveCLI(ctx, db, 999); err == nil {
		t.Fatal("an unknown device")
	}
}

func currentIndex(st devState) int {
	for i, d := range st.Devices {
		if d.Current {
			return i
		}
	}
	return 0
}

// The sessions that are open when the confirmation is turned on stay (and are taken as approved) or are cut, as the administrator chose.
func TestDevicesGateKeepsOrCutsOpenSessions(t *testing.T) {
	base, _ := startAccountsServer(t)
	desk := deviceClient(t, base, token('D'), "windows", "PC")
	if code, _ := desk.login("admin", "a-long-test-password", nil); code != 200 {
		t.Fatal(code)
	}
	// a second computer that signed in before and goes on with its own device token
	other := deviceClient(t, base, token('E'), "linux", "Box")
	if code, _ := other.login("admin", "a-long-test-password", nil); code != 200 {
		t.Fatal(code)
	}
	// an old client: signed in with no device at all
	old := mustLogin(t, base, "admin", "a-long-test-password")

	if code := desk.do("PUT", "/api/devices/gate", map[string]any{"enabled": true, "keep_sessions": false, "confirm": true}, nil); code != 200 {
		t.Fatalf("gate on: %d", code)
	}
	if code := desk.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatalf("the one that asked stays: %d", code)
	}
	for name, c := range map[string]*apiClient{"other": other, "old": old} {
		if code := c.do("GET", "/api/me", nil, nil); code != http.StatusUnauthorized {
			t.Fatalf("%s is signed out: %d", name, code)
		}
	}
	// and with the choice to keep: turn it off, sign in again, turn it on
	if code := desk.do("PUT", "/api/devices/gate", map[string]any{"enabled": false, "confirm": true}, nil); code != 200 {
		t.Fatal(code)
	}
	other = deviceClient(t, base, token('E'), "linux", "Box")
	if code, _ := other.login("admin", "a-long-test-password", nil); code != 200 {
		t.Fatal(code)
	}
	if code := desk.do("PUT", "/api/devices/gate", map[string]any{"enabled": true, "keep_sessions": true, "confirm": true}, nil); code != 200 {
		t.Fatal(code)
	}
	if code := other.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatalf("a device that was in stays in: %d", code)
	}
}

var _ = store.DeviceApproved
