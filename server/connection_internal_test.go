package server

import (
	"net/http"
	"net/http/httptest"
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
