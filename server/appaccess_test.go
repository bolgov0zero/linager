package server

import (
	"net/netip"
	"testing"
)

func mustApp(t *testing.T, cfg AppAccessConfig) *accessRules {
	t.Helper()
	_, ar, err := compileAppAccess(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return ar
}

func TestCompileAppAccess(t *testing.T) {
	norm, _, err := compileAppAccess(AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.5"}, {CIDR: "10.0.0.5/32"}, {CIDR: "192.168.1.9/24", Note: " office "}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(norm.White) != 2 || norm.White[0].CIDR != "10.0.0.5/32" || norm.White[1].CIDR != "192.168.1.0/24" || norm.White[1].Note != "office" {
		t.Fatalf("normalised: %+v", norm.White)
	}
	for name, cfg := range map[string]AppAccessConfig{
		"mode":      {Mode: "maybe"},
		"address":   {Mode: AccessWhite, White: []AccessRule{{CIDR: "nope"}}},
		"host name": {Mode: AccessBlack, Black: []AccessRule{{CIDR: "home.example.com"}}},
	} {
		if _, _, err := compileAppAccess(cfg); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestAppAccessDecisions(t *testing.T) {
	a := func(s string) netip.Addr { return netip.MustParseAddr(s) }
	white := mustApp(t, AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.0/24"}}})
	if !white.decide(a("10.0.0.7"), "").Allowed || white.decide(a("203.0.113.1"), "").Allowed {
		t.Error("white list")
	}
	// a forwarding header names nobody here: the address of the connection is judged
	if white.decide(a("203.0.113.1"), "10.0.0.7").Allowed {
		t.Error("X-Forwarded-For must not help")
	}
	black := mustApp(t, AppAccessConfig{Mode: AccessBlack, Black: []AccessRule{{CIDR: "203.0.113.0/24"}}})
	if black.decide(a("203.0.113.9"), "").Allowed || !black.decide(a("10.0.0.7"), "").Allowed {
		t.Error("black list")
	}
	if !mustApp(t, AppAccessConfig{Mode: AccessOff}).decide(a("203.0.113.9"), "").Allowed {
		t.Error("off")
	}
}

func TestAppAccessProblems(t *testing.T) {
	a := netip.MustParseAddr("10.0.0.7")
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessWhite}), a, true); len(p) != 2 || p[0].Code != problemWhiteEmpty || p[1].Code != problemLockout {
		t.Errorf("an empty white list locks the caller out: %+v", p)
	}
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.0/8"}}}), a, true); len(p) != 0 {
		t.Errorf("the caller is listed: %+v", p)
	}
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessBlack, Black: []AccessRule{{CIDR: "10.0.0.7"}}}), a, true); len(p) != 1 || p[0].Code != problemLockout {
		t.Errorf("the caller is blacklisted: %+v", p)
	}
	// a request through the admin panel is not judged by the lists of the port
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessBlack, Black: []AccessRule{{CIDR: "10.0.0.7"}}}), a, false); len(p) != 0 {
		t.Errorf("not direct: %+v", p)
	}
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessOff}), a, true); len(p) != 0 {
		t.Errorf("off: %+v", p)
	}
}

// Until the list of the port is saved, the port follows the admin panel; after that the lists are apart.
func TestAppAccessFollowsAdminUntilSaved(t *testing.T) {
	g := testGuard()
	_, ar, err := compileAccess(AccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.0/24"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.snap.Store(&accessSnapshot{cfg: AccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.0/24"}}}, rules: ar, configured: true})
	out, in := netip.MustParseAddr("203.0.113.1"), netip.MustParseAddr("10.0.0.7")
	if g.decideApp(out).Allowed || !g.decideApp(in).Allowed {
		t.Fatal("not saved: the port follows the admin white list")
	}
	if cfg, saved := g.AppCurrent(); saved || cfg.Mode != AccessWhite || len(cfg.White) != 1 {
		t.Fatalf("inherited view: %+v %v", cfg, saved)
	}
	cfg, rules, _ := compileAppAccess(AppAccessConfig{Mode: AccessOff})
	g.app.Store(&appAccessSnapshot{cfg: cfg, rules: rules})
	if !g.decideApp(out).Allowed {
		t.Fatal("saved as off: the admin list no longer applies to the port")
	}
}
