package server

import (
	"net/netip"
	"testing"
)

func rules(cidrs ...string) []AccessRule {
	out := []AccessRule{}
	for _, c := range cidrs {
		out = append(out, AccessRule{CIDR: c})
	}
	return out
}

func TestParseAccessRule(t *testing.T) {
	ok := map[string]string{
		"203.0.113.7":        "203.0.113.7/32",
		" 203.0.113.7 ":      "203.0.113.7/32",
		"203.0.113.77/24":    "203.0.113.0/24", // host bits are cleared
		"2001:db8::1":        "2001:db8::1/128",
		"::ffff:203.0.113.7": "203.0.113.7/32", // mapped form is the same address
		"fd00::/8":           "fd00::/8",
	}
	for in, want := range ok {
		got, _, err := ParseAccessRule(AccessRule{CIDR: in})
		if err != nil || got.CIDR != want {
			t.Errorf("%q: got %q err %v, want %q", in, got.CIDR, err, want)
		}
	}
	for _, in := range []string{"", "host.example", "300.1.1.1", "1.2.3.4/33", "1.2.3.4/x", "::ffff:1.2.3.4/120"} {
		if _, _, err := ParseAccessRule(AccessRule{CIDR: in}); err == nil {
			t.Errorf("%q must be refused", in)
		}
	}
	long := make([]rune, maxRuleNote+1)
	for i := range long {
		long[i] = 'я'
	}
	if _, _, err := ParseAccessRule(AccessRule{CIDR: "1.1.1.1", Note: string(long)}); err == nil {
		t.Error("a long comment must be refused")
	}
}

func TestCompileAccessDedupesAndValidates(t *testing.T) {
	cfg, _, err := compileAppAccess(AppAccessConfig{Mode: AccessWhite, White: rules("10.0.0.1", "10.0.0.1/32", "10.0.0.2")}, nil)
	if err != nil || len(cfg.White) != 2 {
		t.Fatalf("got %+v err %v", cfg, err)
	}
	if _, _, err := compileAppAccess(AppAccessConfig{Mode: "allow-all"}, nil); err == nil {
		t.Error("unknown mode must be refused")
	}
	var many []AccessRule
	for i := 0; i <= maxAccessRules; i++ {
		many = append(many, AccessRule{CIDR: netip.AddrFrom4([4]byte{10, byte((i >> 8) & 0xff), byte(i & 0xff), 1}).String()})
	}
	if _, _, err := compileAppAccess(AppAccessConfig{Mode: AccessWhite, White: many}, nil); err == nil {
		t.Error("too many entries must be refused")
	}
}

func TestDecide(t *testing.T) {
	a := netip.MustParseAddr
	white := mustApp(t, AppAccessConfig{Mode: AccessWhite, White: rules("10.0.0.0/8", "2001:db8::/32")})
	for addr, want := range map[string]struct {
		ok     bool
		reason string
	}{
		"10.1.2.3":        {true, reasonInWhite},
		"::ffff:10.1.2.3": {true, reasonInWhite}, // the mapped form is the same address
		"2001:db8::5":     {true, reasonInWhite},
		"203.0.113.1":     {false, reasonNotInWhite},
		"2001:dead::1":    {false, reasonNotInWhite},
	} {
		if v := white.decide(a(addr)); v.Allowed != want.ok || v.Reason != want.reason {
			t.Errorf("white %s: %+v", addr, v)
		}
	}
	black := mustApp(t, AppAccessConfig{Mode: AccessBlack, Black: rules("203.0.113.0/24")})
	if v := black.decide(a("203.0.113.9")); v.Allowed || v.Reason != reasonInBlack {
		t.Errorf("black, listed: %+v", v)
	}
	if v := black.decide(a("10.0.0.1")); !v.Allowed || v.Reason != reasonNotInBlack {
		t.Errorf("black, other: %+v", v)
	}
	if v := mustApp(t, AppAccessConfig{Mode: AccessOff}).decide(a("203.0.113.9")); !v.Allowed || v.Reason != reasonOff {
		t.Errorf("off: %+v", v)
	}
	if v := white.decide(netip.Addr{}); v.Allowed || v.Reason != reasonBadAddress {
		t.Errorf("an address that cannot be read is refused: %+v", v)
	}
}
