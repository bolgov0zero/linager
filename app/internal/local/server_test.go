package local

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bolgov0zero/linager/app/internal/profiles"
	"github.com/bolgov0zero/linager/app/internal/remote"
	"github.com/bolgov0zero/linager/app/internal/update"
)

type calls struct{ kind, id, title, arg string }

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
		Assets:   fstest.MapFS{"index.html": {Data: []byte("<html>linager</html>")}},
		Profiles: store,
		OpenWindow: func(kind, id, title, arg string) error {
			c.kind, c.id, c.title, c.arg = kind, id, title, arg
			return nil
		},
		Version: "test",
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
	for _, k := range []string{"note", "files", "manager", "editor"} {
		if code := call(t, "POST", s.URL()+"app/window", map[string]string{"kind": k, "id": "srv-1", "arg": "/etc/hosts"}, nil); code != 200 || win.kind != k || win.arg != "/etc/hosts" {
			t.Fatalf("window %s: %d %+v", k, code, win)
		}
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

// The session of the panel is kept in the keychain, comes back after a restart (a new Session of the same profile),
// is forgotten when the server says it is gone and on sign-out.
func TestSessionIsKept(t *testing.T) {
	alive := true
	up := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			http.SetCookie(w, &http.Cookie{Name: "linager_session", Value: "tok-1", Path: "/", Secure: true, MaxAge: 604800})
			_, _ = io.WriteString(w, `{"csrf":"c"}`)
		case "/api/logout":
			http.SetCookie(w, &http.Cookie{Name: "linager_session", Value: "", Path: "/", MaxAge: -1})
			_, _ = io.WriteString(w, `{}`)
		case "/api/me":
			if !alive || r.Header.Get("Cookie") != "linager_session=tok-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"csrf":"c","login":"admin"}`)
		}
	}))
	up.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	up.StartTLS()
	t.Cleanup(up.Close)
	store, _ := profiles.Open(filepath.Join(t.TempDir(), "profiles.json"))
	secrets := &profiles.Memory{}
	s, err := New(Options{Profiles: store, Secrets: secrets})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	t.Cleanup(s.Close)
	addr, fp := strings.TrimPrefix(up.URL, "https://"), remote.Fingerprint(up.Certificate().Raw)
	var st struct {
		Profile  struct{ ID string }
		Profiles []struct {
			ID      string
			Session bool
		}
	}
	call(t, "POST", s.URL()+"app/connect", map[string]string{"address": addr, "fingerprint": fp}, &st)
	id := st.Profile.ID
	if code := call(t, "GET", s.URL()+"api/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("not signed in yet: %d", code)
	}
	call(t, "POST", s.URL()+"api/login", map[string]string{}, nil)
	if v, err := secrets.Get("session:" + id); err != nil || v != "tok-1" {
		t.Fatalf("the cookie is kept: %q %v", v, err)
	}

	// a restart: the application disconnects (the jar is gone), connects again and is still signed in
	call(t, "POST", s.URL()+"app/disconnect", nil, nil)
	call(t, "POST", s.URL()+"app/use", map[string]string{"id": id}, &st)
	if len(st.Profiles) != 1 || !st.Profiles[0].Session {
		t.Fatalf("the profile says a session is kept: %+v", st)
	}
	if code := call(t, "GET", s.URL()+"api/me", nil, nil); code != 200 {
		t.Fatalf("the kept session is used again: %d", code)
	}

	// the server ended it (seven idle days): forgotten, so the next start asks for the password and 2FA
	alive = false
	if code := call(t, "GET", s.URL()+"api/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("ended by the server: %d", code)
	}
	if _, err := secrets.Get("session:" + id); err == nil {
		t.Fatal("a session the server no longer knows must be forgotten")
	}

	// sign-out forgets it too
	alive = true
	call(t, "POST", s.URL()+"api/login", map[string]string{}, nil)
	call(t, "POST", s.URL()+"api/logout", map[string]string{}, nil)
	if _, err := secrets.Get("session:" + id); err == nil {
		t.Fatal("sign-out must forget the session")
	}
}

func TestPrefsAreKept(t *testing.T) {
	prefs := profiles.OpenPrefs(filepath.Join(t.TempDir(), "prefs.json"))
	store, _ := profiles.Open(filepath.Join(t.TempDir(), "profiles.json"))
	s, err := New(Options{Assets: fstest.MapFS{"index.html": {Data: []byte("x")}}, Profiles: store, Prefs: prefs, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	t.Cleanup(s.Close)
	base := s.URL()
	if c := call(t, "PUT", base+"app/prefs", map[string]string{"linager.layout": "rows"}, nil); c != 200 {
		t.Fatalf("put: %d", c)
	}
	var got map[string]string
	call(t, "GET", base+"app/prefs", nil, &got)
	if got["linager.layout"] != "rows" {
		t.Fatalf("not kept: %v", got)
	}
	if prefs.All()["linager.layout"] != "rows" {
		t.Fatal("not written to the store")
	}
}

func TestDownloadIsSavedByTheApplication(t *testing.T) {
	s, _, up := start(t)
	addr := strings.TrimPrefix(up.URL, "https://")
	fp := remote.Fingerprint(up.Certificate().Raw)
	dest := filepath.Join(t.TempDir(), "saved.txt")
	ask := ""
	s.o.SaveFile = func(name string) (string, bool, error) {
		ask = name
		return dest, true, nil
	}
	if code := call(t, "POST", s.URL()+"app/connect", map[string]string{"address": addr, "fingerprint": fp}, nil); code != 200 {
		t.Fatalf("connect: %d", code)
	}
	good := map[string]string{"path": "api/servers/abc-1/files/download?path=%2Fetc%2Fhosts", "name": "../hosts"}
	var out struct{ Saved string }
	if code := call(t, "POST", s.URL()+"app/download", good, &out); code != 200 || out.Saved != dest {
		t.Fatalf("download: %d %+v", code, out)
	}
	if ask != "hosts" {
		t.Fatalf("the name offered must be a plain file name, got %q", ask)
	}
	b, err := os.ReadFile(dest)
	if err != nil || !strings.Contains(string(b), `"path":"/api/servers/abc-1/files/download"`) {
		t.Fatalf("file: %q %v", b, err)
	}
	// only the downloads of the file manager can be fetched this way
	for _, p := range []string{"api/users", "api/servers/x/files/content?path=/", "../etc/passwd", "api/servers/x/files/downloadx?a=1"} {
		if code := call(t, "POST", s.URL()+"app/download", map[string]string{"path": p, "name": "x"}, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d", p, code)
		}
	}
	s.o.SaveFile = func(string) (string, bool, error) { return "", false, nil }
	var c struct{ Cancelled bool }
	if code := call(t, "POST", s.URL()+"app/download", good, &c); code != 200 || !c.Cancelled {
		t.Fatalf("cancel: %d %+v", code, c)
	}
}

func TestVaultFileGoesThroughNativeDialogs(t *testing.T) {
	prefs := profiles.OpenPrefs(filepath.Join(t.TempDir(), "prefs.json"))
	store, _ := profiles.Open(filepath.Join(t.TempDir(), "profiles.json"))
	dir := t.TempDir()
	src := filepath.Join(dir, "keys.lvault")
	if err := os.WriteFile(src, []byte("LGRK-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "saved.lvault")
	ext := ""
	s, err := New(Options{
		Assets: fstest.MapFS{"index.html": {Data: []byte("x")}}, Profiles: store, Prefs: prefs, Version: "test",
		PickFile: func(e string) (string, bool, error) { ext = e; return src, true, nil },
		SaveFile: func(name string) (string, bool, error) { return dest, true, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	t.Cleanup(s.Close)
	base := s.URL()

	var got struct{ Name, Data string }
	if c := call(t, "POST", base+"app/vault/open", nil, &got); c != 200 || got.Name != "keys.lvault" || got.Data != base64.StdEncoding.EncodeToString([]byte("LGRK-bytes")) || ext != ".lvault" {
		t.Fatalf("open: %d %+v ext=%q", c, got, ext)
	}
	if c := call(t, "POST", base+"app/vault/save", map[string]string{"name": "../x.lvault", "data": base64.StdEncoding.EncodeToString([]byte("sealed"))}, nil); c != 200 {
		t.Fatalf("save: %d", c)
	}
	b, err := os.ReadFile(dest)
	st, _ := os.Stat(dest)
	if err != nil || string(b) != "sealed" || st.Mode().Perm() != 0o600 {
		t.Fatalf("saved file: %q %v %v", b, err, st)
	}
	// a file that is too big is not read, and an empty or broken save is refused
	big := filepath.Join(dir, "big.lvault")
	_ = os.WriteFile(big, make([]byte, maxVault+1), 0o600)
	s.o.PickFile = func(string) (string, bool, error) { return big, true, nil }
	if c := call(t, "POST", base+"app/vault/open", nil, nil); c != http.StatusRequestEntityTooLarge {
		t.Fatalf("too big: %d", c)
	}
	if c := call(t, "POST", base+"app/vault/save", map[string]string{"name": "x", "data": "!!"}, nil); c != http.StatusBadRequest {
		t.Fatalf("bad data: %d", c)
	}
	s.o.PickFile = func(string) (string, bool, error) { return "", false, nil }
	var cancelled struct{ Cancelled bool }
	if c := call(t, "POST", base+"app/vault/open", nil, &cancelled); c != 200 || !cancelled.Cancelled {
		t.Fatalf("cancel: %d %+v", c, cancelled)
	}
}

func TestUpdateEndpoints(t *testing.T) {
	store, _ := profiles.Open(filepath.Join(t.TempDir(), "profiles.json"))
	// no updater (the headless run): the build cannot update itself
	s, err := New(Options{Assets: fstest.MapFS{"index.html": {Data: []byte("x")}}, Profiles: store, Version: "0.10.0-beta.3"})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	t.Cleanup(s.Close)
	var info struct {
		Supported bool   `json:"supported"`
		Reason    string `json:"reason"`
		Newer     bool   `json:"newer"`
		Channel   string `json:"channel"`
	}
	if c := call(t, "POST", s.URL()+"app/update/check", nil, &info); c != 200 || info.Supported || info.Reason != "headless" || info.Channel != "beta" {
		t.Fatalf("check: %d %+v", c, info)
	}
	if c := call(t, "POST", s.URL()+"app/update/install", nil, nil); c != http.StatusNotImplemented {
		t.Fatalf("install without an updater: %d", c)
	}
	var st struct{ Phase string }
	if c := call(t, "GET", s.URL()+"app/update/status", nil, &st); c != 200 || st.Phase != "idle" {
		t.Fatalf("status: %d %+v", c, st)
	}

	// an updater with nothing to offer
	rel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "[]") }))
	t.Cleanup(rel.Close)
	up, _ := update.New("0.10.0-beta.3", t.TempDir(), func() {})
	up.API, up.AllowHTTP = rel.URL, true
	s2, err := New(Options{Assets: fstest.MapFS{"index.html": {Data: []byte("x")}}, Profiles: store, Update: up, Version: "0.10.0-beta.3"})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s2.Serve() }()
	t.Cleanup(s2.Close)
	if c := call(t, "POST", s2.URL()+"app/update/check", nil, &info); c != 200 || !info.Supported || info.Newer {
		t.Fatalf("check: %d %+v", c, info)
	}
	if c := call(t, "GET", s2.URL()+"app/update/info", nil, &info); c != 200 || !info.Supported {
		t.Fatalf("info: %d %+v", c, info)
	}
	// the token still guards these paths
	resp, err := http.Get(strings.TrimSuffix(s2.URL(), strings.SplitN(strings.TrimPrefix(s2.URL(), "http://"), "/", 2)[1]) + "app/update/info")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			t.Fatal("the update paths need the token")
		}
	}
}
