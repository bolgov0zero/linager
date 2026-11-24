package remote

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func fingerprintOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return Fingerprint(srv.Certificate().Raw)
}

func addrOf(srv *httptest.Server) string { return strings.TrimPrefix(srv.URL, "https://") }

func newTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func TestProbeAndPin(t *testing.T) {
	srv := newTLSServer(t, http.NotFoundHandler())
	fp, err := Probe(context.Background(), addrOf(srv))
	if err != nil || fp != fingerprintOf(t, srv) || !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("probe: %q %v", fp, err)
	}
	ok := &http.Client{Transport: &http.Transport{TLSClientConfig: PinnedTLS(fp)}}
	if resp, err := ok.Get(srv.URL); err != nil {
		t.Fatalf("the pinned certificate must be accepted: %v", err)
	} else {
		_ = resp.Body.Close()
	}
	bad := &http.Client{Transport: &http.Transport{TLSClientConfig: PinnedTLS("SHA256:wrong")}}
	if _, err := bad.Get(srv.URL); err == nil || !strings.Contains(err.Error(), ErrPin.Error()) {
		t.Fatalf("another certificate must be refused: %v", err)
	}
	if _, err := Probe(context.Background(), "127.0.0.1:1"); err == nil {
		t.Fatal("a closed port is an error")
	}
}

func proxyFor(t *testing.T, srv *httptest.Server) *httptest.Server {
	t.Helper()
	s, err := NewSession(Target{Addr: addrOf(srv), Fingerprint: fingerprintOf(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	front := httptest.NewServer(s.Handler())
	t.Cleanup(front.Close)
	return front
}

// The session cookie stays in the application: the web view never gets it and the server gets it back on every call.
func TestCookiesStayInTheApplication(t *testing.T) {
	var seen []string
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"|"+r.Header.Get("Cookie")+"|"+r.Header.Get("X-Forwarded-For"))
		if r.URL.Path == "/api/login" {
			http.SetCookie(w, &http.Cookie{Name: "linager_session", Value: "s3cret", Path: "/", Secure: true, HttpOnly: true})
		}
		_, _ = io.WriteString(w, "ok")
	}))
	front := proxyFor(t, srv)

	req, _ := http.NewRequest("POST", front.URL+"/api/login", nil)
	req.Header.Set("Cookie", "evil=1") // whatever the web view sends is not trusted
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("Set-Cookie"); got != "" {
		t.Fatalf("the web view must not see the cookie: %q", got)
	}
	resp, err = http.Get(front.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(seen) != 2 || seen[0] != "/api/login||" || seen[1] != "/api/me|linager_session=s3cret|" {
		t.Fatalf("what the server saw (the jar cookie, no forwarding header): %q", seen)
	}
}

func TestLiveStreamIsNotBuffered(t *testing.T) {
	release := make(chan struct{})
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: status\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	front := proxyFor(t, srv)
	defer close(release)
	resp, err := http.Get(front.URL + "/api/live")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(resp.Body).ReadString('\n'); got <- line }()
	select {
	case line := <-got:
		if line != "event: status\n" {
			t.Fatalf("first line: %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the first event did not arrive while the stream was open")
	}
}

// A WebSocket through the proxy: the server checks that Origin and Host agree (as the Linager server does), so the
// proxy must not pass the local origin on.
func TestWebSocketThroughTheProxy(t *testing.T) {
	srv := newTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil) // refuses a foreign Origin
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		for {
			typ, b, err := c.Read(r.Context())
			if err != nil {
				return
			}
			if err := c.Write(r.Context(), typ, append([]byte("echo:"), b...)); err != nil {
				return
			}
		}
	}))
	front := proxyFor(t, srv)
	u, _ := url.Parse(front.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://"+u.Host+"/api/servers/x/stream", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://127.0.0.1:1"}}})
	if err != nil {
		t.Fatalf("dial through the proxy: %v", err)
	}
	defer func() { _ = c.CloseNow() }()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("ls")); err != nil {
		t.Fatal(err)
	}
	_, b, err := c.Read(ctx)
	if err != nil || string(b) != "echo:ls" {
		t.Fatalf("echo: %q %v", b, err)
	}
}

func TestPinMismatchIsReported(t *testing.T) {
	srv := newTLSServer(t, http.NotFoundHandler())
	s, _ := NewSession(Target{Addr: addrOf(srv), Fingerprint: "SHA256:other"})
	front := httptest.NewServer(s.Handler())
	defer front.Close()
	resp, err := http.Get(front.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(b), "pin_mismatch") {
		t.Fatalf("a changed certificate: %d %s", resp.StatusCode, b)
	}
	if _, err := NewSession(Target{Addr: "no-port", Fingerprint: "x"}); err == nil {
		t.Fatal("an address without a port must be refused")
	}
	if !errors.Is(fmt.Errorf("x: %w", ErrPin), ErrPin) {
		t.Fatal("ErrPin must be comparable")
	}
}
