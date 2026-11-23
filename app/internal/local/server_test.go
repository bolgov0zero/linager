package local

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bolgov0zero/linager/app/internal/profiles"
	"github.com/bolgov0zero/linager/app/internal/remote"
)

type calls struct{ kind, id, title string }

func start(t *testing.T) (*Server, *calls, *httptest.Server) {
	t.Helper()
	up := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" {
			http.SetCookie(w, &http.Cookie{Name: "linager_session", Value: "x", Path: "/", Secure: true})
		}
		_, _ = io.WriteString(w, `{"path":"`+r.URL.Path+`","cookie":"`+r.Header.Get("Cookie")+`"}`)
	}))
	up.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	up.StartTLS()
	t.Cleanup(up.Close)
	store, err := profiles.Open(filepath.Join(t.TempDir(), "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &calls{}
	s, err := New(Options{
		Assets:     fstest.MapFS{"index.html": {Data: []byte("<html>linager</html>")}},
		Profiles:   store,
		OpenWindow: func(kind, id, title string) error { c.kind, c.id, c.title = kind, id, title; return nil },
		Version:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	t.Cleanup(s.Close)
	return s, c, up
}

func call(t *testing.T, method, url string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestEveryPathNeedsTheToken(t *testing.T) {
	s, _, _ := start(t)
	base := strings.TrimSuffix(s.URL(), s.token+"/")
	for _, p := range []string{"", "app/state", "wrong/app/state", s.token + "x/app/state"} {
		if code := call(t, "GET", base+p, nil, nil); code != http.StatusNotFound {
			t.Errorf("/%s without the token: %d", p, code)
		}
	}
	if code := call(t, "GET", s.URL()+"app/state", nil, nil); code != 200 {
		t.Fatalf("with the token: %d", code)
	}
	resp, err := http.Get(s.URL())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(b), "linager") || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("the interface is served: %q %v", b, resp.Header)
	}
	// DNS rebinding: a request that names another host is refused even with the token
	req, _ := http.NewRequest("GET", s.URL()+"app/state", nil)
	req.Host = "evil.example.com:80"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a foreign Host: %d", resp.StatusCode)
	}
}

func TestConnectionFlow(t *testing.T) {
	s, win, up := start(t)
	addr := strings.TrimPrefix(up.URL, "https://")
	fp := remote.Fingerprint(up.Certificate().Raw)

	if code := call(t, "GET", s.URL()+"api/me", nil, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("not connected: %d", code)
	}
	var pr struct{ Fingerprint string }
	if code := call(t, "POST", s.URL()+"app/probe", map[string]string{"address": addr}, &pr); code != 200 || pr.Fingerprint != fp {
		t.Fatalf("probe: %d %+v", code, pr)
	}
	if code := call(t, "POST", s.URL()+"app/probe", map[string]string{"address": "127.0.0.1:1"}, nil); code != http.StatusBadGateway {
		t.Fatalf("probe of a closed port: %d", code)
	}
	var er struct{ Error string }
	if code := call(t, "POST", s.URL()+"app/connect", map[string]string{"address": addr, "fingerprint": "SHA256:wrong"}, &er); code != http.StatusConflict || er.Error != "pin_mismatch" {
		t.Fatalf("a wrong fingerprint: %d %+v", code, er)
	}
	var st struct {
		Connected bool
		Profile   struct{ ID, Address string }
		Profiles  []struct{ ID string }
	}
	if code := call(t, "POST", s.URL()+"app/connect", map[string]string{"address": addr, "fingerprint": fp, "name": "lab"}, &st); code != 200 || !st.Connected || st.Profile.Address != addr || len(st.Profiles) != 1 {
		t.Fatalf("connect: %d %+v", code, st)
	}
	// the API goes through the proxy, the cookie of the server stays inside
	req, _ := http.NewRequest("POST", s.URL()+"api/login", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.Header.Get("Set-Cookie") != "" {
		t.Fatal("the web view must not get the session cookie")
	}
	var echo struct{ Path, Cookie string }
	if code := call(t, "GET", s.URL()+"api/dashboard", nil, &echo); code != 200 || echo.Path != "/api/dashboard" || echo.Cookie != "linager_session=x" {
		t.Fatalf("proxied call: %d %+v", code, echo)
	}

	// the keychain is for the connected profile only
	id := st.Profile.ID
	if code := call(t, "PUT", s.URL()+"app/secret", map[string]string{"profile": "other", "password": "x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("a secret of another profile: %d", code)
	}
	if code := call(t, "PUT", s.URL()+"app/secret", map[string]string{"profile": id, "password": "pw-1"}, nil); code != 200 {
		t.Fatalf("save a secret: %d", code)
	}
	var sec struct{ Password string }
	if code := call(t, "GET", s.URL()+"app/secret?profile="+id, nil, &sec); code != 200 || sec.Password != "pw-1" {
		t.Fatalf("read the secret: %d %+v", code, sec)
	}
	if code := call(t, "GET", s.URL()+"app/secret?profile=other", nil, nil); code != http.StatusNotFound {
		t.Fatalf("the secret of another profile: %d", code)
	}

	// windows
	if code := call(t, "POST", s.URL()+"app/window", map[string]string{"kind": "terminal", "id": "srv-1", "title": "web-01"}, nil); code != 200 || win.kind != "terminal" || win.id != "srv-1" || win.title != "web-01" {
		t.Fatalf("window: %d %+v", code, win)
	}
	if code := call(t, "POST", s.URL()+"app/window", map[string]string{"kind": "rm -rf", "id": "x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("an unknown kind of window: %d", code)
	}

	// disconnect and use the saved profile again; forgetting removes the password too
	if code := call(t, "POST", s.URL()+"app/disconnect", nil, &st); code != 200 || st.Connected {
		t.Fatalf("disconnect: %d %+v", code, st)
	}
	if code := call(t, "POST", s.URL()+"app/use", map[string]string{"id": id}, &st); code != 200 || !st.Connected {
		t.Fatalf("use: %d %+v", code, st)
	}
	if code := call(t, "POST", s.URL()+"app/forget", map[string]string{"id": id}, &st); code != 200 || len(st.Profiles) != 0 {
		t.Fatalf("forget: %d %+v", code, st)
	}
	if _, err := s.o.Secrets.Get(id); err == nil {
		t.Fatal("the saved password must go with the profile")
	}
}
