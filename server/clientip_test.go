package server

import (
	"net/http/httptest"
	"testing"
)

// The address of a client is the address of the connection; a header that claims another one is not believed.
func TestClientIPIsTheAddressOfTheConnection(t *testing.T) {
	for remote, want := range map[string]string{"203.0.113.5:1234": "203.0.113.5", "[2001:db8::7]:99": "2001:db8::7", "[::ffff:10.1.2.3]:5": "10.1.2.3"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", "6.6.6.6")
		if got := clientIP(r); got != want {
			t.Errorf("%s: got %s want %s", remote, got, want)
		}
	}
}
