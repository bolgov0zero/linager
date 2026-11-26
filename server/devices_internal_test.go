package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestClientInfoIsReadAndLimited(t *testing.T) {
	r, _ := http.NewRequest("GET", "/api/me", nil)
	r.Header.Set(headerClient, "platform=iOS&app=0.1.0&os=27.1&model=iPhone18%2C1")
	got := clientInfo(r)
	if got.Platform != "ios" || got.AppVersion != "0.1.0" || got.OSVersion != "27.1" || got.Model != "iPhone18,1" {
		t.Fatalf("%+v", got)
	}
	r.Header.Set(headerClient, "platform=plan9&app="+strings.Repeat("x", 200))
	got = clientInfo(r)
	if got.Platform != "other" || len([]rune(got.AppVersion)) != 64 {
		t.Fatalf("an unknown platform is \"other\" and a long value is cut: %+v", got)
	}
	r.Header.Set(headerClient, "%zz")
	if got := clientInfo(r); got != (clientInfoZero) {
		t.Fatalf("garbage says nothing: %+v", got)
	}
}

var clientInfoZero = clientInfo(&http.Request{Header: http.Header{}})

func TestDeviceTokenMustLookLikeOne(t *testing.T) {
	good := strings.Repeat("A", 43)
	for tok, want := range map[string]bool{good: true, "": false, "short": false, strings.Repeat("A", 200): false, strings.Repeat("!", 43): false} {
		r, _ := http.NewRequest("GET", "/", nil)
		r.Header.Set(headerDevice, tok)
		if _, ok := deviceToken(r); ok != want {
			t.Errorf("%q: %v", tok, ok)
		}
	}
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set(headerDevice, good)
	h, _ := deviceToken(r)
	if string(h) != string(HashToken(good)) {
		t.Fatal("the token is kept as its hash")
	}
}

func TestTheGateIsReadFromTheSetting(t *testing.T) {
	if !parseGate("").IsZero() {
		t.Fatal("empty is off")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if got := parseGate(now.Format(time.RFC3339Nano)); !got.Equal(now) {
		t.Fatalf("%v", got)
	}
	if parseGate("garbage").IsZero() {
		t.Fatal("a value that cannot be read is taken as on, the safe side")
	}
}

func TestAPendingDeviceMayOnlyAskWhoItIsAndSignOut(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         bool
	}{{"GET", "/api/me", true}, {"POST", "/api/logout", true}, {"GET", "/api/dashboard", false}, {"POST", "/api/me", false}, {"GET", "/api/devices", false}, {"GET", "/api/live", false}} {
		r, _ := http.NewRequest(c.method, c.path, nil)
		if pendingMay(r) != c.want {
			t.Errorf("%s %s", c.method, c.path)
		}
	}
}
