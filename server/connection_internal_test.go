package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestSupportsMove(t *testing.T) {
	for v, want := range map[string]bool{
		"": false, "0.9.1": false, "0.10.0-beta.1": true, "0.10.0-beta.2": true, "0.10.0": true, "1.0.0": true,
		"0.10.0-alpha.1": false, "dev": true,
	} {
		if got := supportsMove(v); got != want {
			t.Errorf("supportsMove(%q) = %v, want %v", v, got, want)
		}
	}
}

// The application port is a direct connection: what a reverse proxy adds must not reach the handler, otherwise a
// client could name its own address and walk around the access restriction.
func TestStripForwarded(t *testing.T) {
	var got http.Header
	h := stripForwarded(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Header.Clone() }))
	r := httptest.NewRequest("GET", "/api/me", nil)
	for _, k := range []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Port", "X-Real-Ip", "Forwarded"} {
		r.Header.Set(k, "203.0.113.9")
	}
	r.Header.Set("User-Agent", "linager-app")
	h.ServeHTTP(httptest.NewRecorder(), r)
	for _, k := range []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Port", "X-Real-Ip", "Forwarded"} {
		if got.Get(k) != "" {
			t.Errorf("%s was not removed", k)
		}
	}
	if got.Get("User-Agent") != "linager-app" {
		t.Error("other headers must stay")
	}
}

// "Only through a trusted proxy" cannot apply to the application port (a direct connection by design): it used to
// refuse every sign-in of the application with a plain 403 on a server that had that restriction on.
func TestAccessOnTheApplicationPort(t *testing.T) {
	_, ar, err := compileAccess(AccessConfig{Mode: AccessBlack, OnlyProxy: true, Black: []AccessRule{{CIDR: "203.0.113.9"}}, Proxies: []AccessRule{{CIDR: "10.0.0.1"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	peer := netip.MustParseAddr("198.51.100.7")
	if v := ar.decide(peer, ""); v.Allowed || v.Reason != reasonOnlyProxy {
		t.Fatalf("the panel: a direct connection is refused: %+v", v)
	}
	if v := ar.decideDirect(peer); !v.Allowed {
		t.Fatalf("the application port: only_proxy does not apply: %+v", v)
	}
	if v := ar.decideDirect(netip.MustParseAddr("203.0.113.9")); v.Allowed || v.Reason != reasonInBlack {
		t.Fatalf("the black list still judges the real address: %+v", v)
	}
	_, white, _ := compileAccess(AccessConfig{Mode: AccessWhite, OnlyProxy: true, White: []AccessRule{{CIDR: "198.51.100.0/24"}}}, nil)
	if v := white.decideDirect(peer); !v.Allowed || v.Reason != reasonInWhite {
		t.Fatalf("the white list: %+v", v)
	}
	if v := white.decideDirect(netip.MustParseAddr("192.0.2.1")); v.Allowed {
		t.Fatalf("an address outside the white list: %+v", v)
	}
	if !ar.onlyProxy {
		t.Fatal("decideDirect must not change the rules")
	}
}
