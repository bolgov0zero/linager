package server

import (
	"net/netip"
	"testing"
)

func mustApp(t *testing.T, cfg AppAccessConfig) *accessRules {
	t.Helper()
	_, ar, err := compileAppAccess(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ar
}

func TestCompileAppAccess(t *testing.T) {
	norm, _, err := compileAppAccess(AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.5"}, {CIDR: "10.0.0.5/32"}, {CIDR: "192.168.1.9/24", Note: " office "}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(norm.White) != 2 || norm.White[0].CIDR != "10.0.0.5/32" || norm.White[1].CIDR != "192.168.1.0/24" || norm.White[1].Note != "office" {
		t.Fatalf("normalised: %+v", norm.White)
	}
	for name, cfg := range map[string]AppAccessConfig{
		"mode":    {Mode: "maybe"},
		"address": {Mode: AccessWhite, White: []AccessRule{{CIDR: "nope"}}},
	} {
		if _, _, err := compileAppAccess(cfg, nil); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestAppAccessDecisions(t *testing.T) {
	a := func(s string) netip.Addr { return netip.MustParseAddr(s) }
	white := mustApp(t, AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.0/24"}}})
	if !white.decide(a("10.0.0.7")).Allowed || white.decide(a("203.0.113.1")).Allowed {
		t.Error("white list")
	}
	black := mustApp(t, AppAccessConfig{Mode: AccessBlack, Black: []AccessRule{{CIDR: "203.0.113.0/24"}}})
	if black.decide(a("203.0.113.9")).Allowed || !black.decide(a("10.0.0.7")).Allowed {
		t.Error("black list")
	}
	if !mustApp(t, AppAccessConfig{Mode: AccessOff}).decide(a("203.0.113.9")).Allowed {
		t.Error("off")
	}
}

func TestAppAccessProblems(t *testing.T) {
	a := netip.MustParseAddr("10.0.0.7")
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessWhite}), a); len(p) != 2 || p[0].Code != problemWhiteEmpty || p[1].Code != problemLockout {
		t.Errorf("an empty white list locks the caller out: %+v", p)
	}
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.0/8"}}}), a); len(p) != 0 {
		t.Errorf("the caller is listed: %+v", p)
	}
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessBlack, Black: []AccessRule{{CIDR: "10.0.0.7"}}}), a); len(p) != 1 || p[0].Code != problemLockout {
		t.Errorf("the caller is blacklisted: %+v", p)
	}
	if p := appProblems(mustApp(t, AppAccessConfig{Mode: AccessOff}), a); len(p) != 0 {
		t.Errorf("off: %+v", p)
	}
}

// Nothing saved: the port is open. A saved list judges the connections, and the lists of the removed admin panel
// restriction are carried over once.
func TestAppAccessNothingSavedIsOpen(t *testing.T) {
	g := testGuard()
	out := netip.MustParseAddr("203.0.113.1")
	if v := g.decideApp(out); !v.Allowed || v.Reason != reasonOff {
		t.Fatalf("nothing saved: %+v", v)
	}
	if cfg := g.AppCurrent(); cfg.Mode != AccessOff || len(cfg.White) != 0 {
		t.Fatalf("view: %+v", cfg)
	}
	cfg, rules, _ := compileAppAccess(AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: "10.0.0.0/24"}}}, nil)
	g.app.Store(&appAccessSnapshot{cfg: cfg, rules: rules})
	if g.decideApp(out).Allowed || !g.decideApp(netip.MustParseAddr("10.0.0.7")).Allowed {
		t.Fatal("a saved white list judges the connections")
	}
}

// A host name in the list stands for the addresses it points to; a name without an address cannot be applied.
func TestAppAccessHostNames(t *testing.T) {
	cfg := AppAccessConfig{Mode: AccessWhite, White: []AccessRule{{CIDR: " Home.Example.ORG. ", Note: "home"}, {CIDR: "10.0.0.0/8"}}}
	norm, ar, err := compileAppAccess(cfg, hostAddrs{"home.example.org": {netip.MustParseAddr("203.0.113.50")}})
	if err != nil {
		t.Fatal(err)
	}
	if norm.White[0].CIDR != "home.example.org" || len(ar.hosts) != 1 || len(ar.unresolved) != 0 {
		t.Fatalf("normalised: %+v hosts %v unresolved %v", norm.White, ar.hosts, ar.unresolved)
	}
	if !ar.decide(netip.MustParseAddr("203.0.113.50")).Allowed || ar.decide(netip.MustParseAddr("203.0.113.51")).Allowed {
		t.Error("the address of the name is allowed, a neighbour is not")
	}
	if got := appHostNames(cfg); len(got) != 1 || got[0] != "home.example.org" {
		t.Errorf("appHostNames: %v", got)
	}
	// no address known: the name is reported and the config cannot be applied
	_, unres, _ := compileAppAccess(cfg, nil)
	p := appProblems(unres, netip.MustParseAddr("10.0.0.7"))
	if len(p) != 1 || p[0].Code != problemHostNoAddr || p[0].Host != "home.example.org" {
		t.Errorf("an unresolved name: %+v", p)
	}
}
