// Package local is the server inside the application that the windows talk to. It listens on the loopback address
// only, every path starts with a random token (another program or web page that finds the port cannot use it),
// and it does three things: it serves the interface, it proxies the API of the Linager server over the pinned
// connection (internal/remote), and it offers the few native things a web page cannot do (profiles, the keychain,
// opening a window).
package local

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bolgov0zero/linager/app/internal/autostart"
	"github.com/bolgov0zero/linager/app/internal/profiles"
	"github.com/bolgov0zero/linager/app/internal/remote"
	"github.com/bolgov0zero/linager/app/internal/tray"
	"github.com/bolgov0zero/linager/app/internal/update"
)

// Options configure the server.
type Options struct {
	Assets   fs.FS // the built interface (index.html at the root)
	Profiles *profiles.Store
	// Prefs keeps the choices of the interface (theme, layout, section); nil keeps none.
	Prefs   *profiles.Prefs
	Secrets profiles.Secrets
	// Debug logs every request (without the token), for development.
	Debug bool
	// SaveFile asks where to save a downloaded file (a native dialog) and returns the path; ok is false when it was
	// cancelled. nil in the headless run: the page downloads by itself then.
	SaveFile func(name string) (path string, ok bool, err error)
	// PickFile asks which file to open (a native dialog) and returns its path; ok is false when it was cancelled. ext is the
	// extension offered (".lvault"). nil in the headless run: the page opens the file by itself then.
	PickFile func(ext string) (path string, ok bool, err error)
	// OpenWindow opens (or raises) a native window of a kind ("terminal") for a server; nil in the headless run.
	OpenWindow func(kind, id, title, arg string) error
	// Update checks for and installs a new version of the application; nil in the headless run (nothing to replace).
	Update *update.Updater
	// Tray keeps the connections of the tray; OnTray is called when what the tray shows may have changed (a connection, a
	// server, the entries); AutostartExe is the program the start at login launches (empty: the setting is not offered).
	Tray         *tray.Store
	OnTray       func()
	AutostartExe string
	// Fullscreen reads (set nil) or changes the full screen state of the window the person is working in; nil in the headless run.
	Fullscreen func(set *bool) (bool, error)
	// FitWindow grows or shrinks the window the person is working in by dw, dh (in the units of the page), within the screen; nil in the headless run.
	FitWindow func(dw, dh int) error
	// VaultMemo is the file that remembers the chosen vault file for seven days; empty keeps nothing.
	VaultMemo string
	Version   string
}

// Server is the local server.
type Server struct {
	o     Options
	vault *vaultMemo
	token string
	ln    net.Listener
	srv   *http.Server

	mu   sync.Mutex
	sess *remote.Session
	prof profiles.Profile
}

// New starts listening on a free loopback port.
func New(o Options) (*Server, error) {
	if o.Secrets == nil {
		o.Secrets = &profiles.Memory{}
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{o: o, token: hex.EncodeToString(b), ln: ln, vault: newVaultMemo(o.VaultMemo)}
	if o.Update != nil {
		// the checks use the GitHub token of the person when there is one
		o.Update.Token = func() string {
			v, _ := s.o.Secrets.Get(githubTokenKey)
			return v
		}
	}
	s.srv = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
	return s, nil
}

// Serve runs until Close.
func (s *Server) Serve() error {
	err := s.srv.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close stops the server.
func (s *Server) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
	s.mu.Lock()
	if s.sess != nil {
		s.sess.Close()
	}
	s.mu.Unlock()
}

// URL is the address a window opens, with a trailing slash.
func (s *Server) URL() string { return "http://" + s.ln.Addr().String() + "/" + s.token + "/" }

func (s *Server) handler() http.Handler {
	inner := http.NewServeMux()
	inner.HandleFunc("GET /app/state", s.state)
	inner.HandleFunc("POST /app/probe", s.probe)
	inner.HandleFunc("POST /app/connect", s.connect)
	inner.HandleFunc("POST /app/use", s.use)
	inner.HandleFunc("POST /app/disconnect", s.disconnect)
	inner.HandleFunc("POST /app/forget", s.forget)
	inner.HandleFunc("POST /app/login", s.rememberLogin)
	inner.HandleFunc("GET /app/secret", s.secretGet)
	inner.HandleFunc("PUT /app/secret", s.secretPut)
	inner.HandleFunc("DELETE /app/secret", s.secretDelete)
	inner.HandleFunc("POST /app/window", s.window)
	inner.HandleFunc("POST /app/download", s.download)
	inner.HandleFunc("POST /app/vault/open", s.vaultOpen)
	inner.HandleFunc("POST /app/vault/save", s.vaultSave)
	inner.HandleFunc("PUT /app/vault/write", s.vaultWrite)
	inner.HandleFunc("GET /app/tray", s.trayGet)
	inner.HandleFunc("PUT /app/tray", s.trayPut)
	inner.HandleFunc("GET /app/autostart", s.autostartGet)
	inner.HandleFunc("PUT /app/autostart", s.autostartPut)
	inner.HandleFunc("GET /app/fullscreen", s.fullscreenGet)
	inner.HandleFunc("PUT /app/fullscreen", s.fullscreenPut)
	inner.HandleFunc("PUT /app/window-fit", s.windowFit)
	inner.HandleFunc("GET /app/vault/last", s.vaultLast)
	inner.HandleFunc("DELETE /app/vault/last", s.vaultForget)
	inner.HandleFunc("GET /app/prefs", s.prefsGet)
	inner.HandleFunc("PUT /app/prefs", s.prefsPut)
	inner.HandleFunc("POST /app/update/check", s.updateCheck)
	inner.HandleFunc("GET /app/update/info", s.updateInfo)
	inner.HandleFunc("POST /app/update/install", s.updateInstall)
	inner.HandleFunc("GET /app/update/status", s.updateStatus)
	inner.HandleFunc("GET /app/update/token", s.tokenGet)
	inner.HandleFunc("PUT /app/update/token", s.tokenPut)
	inner.HandleFunc("DELETE /app/update/token", s.tokenDelete)
	inner.HandleFunc("/api/", s.api)
	if s.o.Assets != nil {
		inner.Handle("/", http.FileServerFS(s.o.Assets))
	}
	prefix := "/" + s.token
	strip := http.StripPrefix(prefix, inner)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// the page is only ever opened by address: a request with another Host is a DNS rebinding attempt
		if h, _, err := net.SplitHostPort(r.Host); err != nil || (h != "127.0.0.1" && h != "localhost") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
			http.NotFound(w, r)
			return
		}
		if s.o.Debug {
			log.Printf("local: %s %s [%s]", r.Method, strings.TrimPrefix(r.URL.Path, prefix), r.UserAgent())
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		strip.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "bad_request")
		return false
	}
	return true
}

type profileView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Fingerprint string `json:"fingerprint"`
	Login       string `json:"login"`
	Saved       bool   `json:"saved"`   // a password is in the keychain
	Session     bool   `json:"session"` // a session of an earlier run is kept: the application can open without asking
}

func (s *Server) view(p profiles.Profile) profileView {
	_, err := s.o.Secrets.Get(p.ID)
	_, serr := s.o.Secrets.Get(sessionKey(p))
	return profileView{ID: p.ID, Name: p.Name, Address: p.Address, Fingerprint: p.Fingerprint, Login: p.Login, Saved: err == nil, Session: serr == nil}
}

func (s *Server) stateBody() map[string]any {
	s.mu.Lock()
	connected, cur := s.sess != nil, s.prof
	s.mu.Unlock()
	list := make([]profileView, 0)
	for _, p := range s.o.Profiles.List() {
		list = append(list, s.view(p))
	}
	st := map[string]any{"version": s.o.Version, "connected": connected, "profiles": list, "native": s.o.OpenWindow != nil}
	if connected {
		st["profile"] = s.view(cur)
	}
	return st
}

// changed tells the tray that its menu may have to change.
func (s *Server) changed() {
	if s.o.OnTray != nil {
		go s.o.OnTray()
	}
}

// Profile is the profile of the connection ("" when there is none).
func (s *Server) Profile() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess == nil {
		return ""
	}
	return s.prof.ID
}

// Get reads a path of the Linager server with the session of the person (the tray reads the dashboard this way).
func (s *Server) Get(ctx context.Context, path string) ([]byte, error) {
	s.mu.Lock()
	sess := s.sess
	s.mu.Unlock()
	if sess == nil {
		return nil, errors.New("not connected")
	}
	resp, err := sess.Fetch(ctx, "/"+path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.stateBody())
}

func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}
	if !decode(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	fp, err := remote.Probe(ctx, strings.TrimSpace(req.Address))
	if err != nil {
		fail(w, http.StatusBadGateway, "unreachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"fingerprint": fp})
}

// open pins a session to a profile and checks that the server answers over the pin.
func (s *Server) open(r *http.Request, p profiles.Profile) error {
	sess, err := remote.NewSession(remote.Target{Addr: p.Address, Fingerprint: p.Fingerprint})
	if err != nil {
		return err
	}
	// the session of an earlier run (the server renews it with every request and ends it after seven idle days)
	if old, err := s.o.Secrets.Get(sessionKey(p)); err == nil {
		sess.RestoreSession(old)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sess.URL("/api/me"), nil)
	resp, err := sess.Client().Do(req)
	if err != nil {
		sess.Close()
		return err
	}
	_ = resp.Body.Close()
	s.mu.Lock()
	if s.sess != nil {
		s.sess.Close()
	}
	s.sess, s.prof = sess, p
	s.mu.Unlock()
	sess.OnSession = func(v string) {
		s.mu.Lock()
		cur := s.prof
		s.mu.Unlock()
		s.keepSession(cur, v)
	}
	return nil
}

// sessionKey is where the session cookie of a profile is kept (the keychain, next to the saved password).
func sessionKey(p profiles.Profile) string { return "session:" + p.ID }

// keepSession stores or forgets the session cookie the server set.
func (s *Server) keepSession(p profiles.Profile, value string) {
	if value == "" {
		_ = s.o.Secrets.Delete(sessionKey(p))
		return
	}
	_ = s.o.Secrets.Set(sessionKey(p), value)
}

func (s *Server) openError(w http.ResponseWriter, err error) {
	if errors.Is(err, remote.ErrPin) || strings.Contains(err.Error(), remote.ErrPin.Error()) {
		fail(w, http.StatusConflict, "pin_mismatch")
		return
	}
	fail(w, http.StatusBadGateway, "unreachable")
}

func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Address     string `json:"address"`
		Fingerprint string `json:"fingerprint"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Address = strings.TrimSpace(req.Address)
	if req.Address == "" || !strings.HasPrefix(req.Fingerprint, "SHA256:") {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	p := profiles.Profile{Name: req.Name, Address: req.Address, Fingerprint: req.Fingerprint}
	if p.Name == "" {
		p.Name = req.Address
	}
	if err := s.open(r, p); err != nil {
		s.openError(w, err)
		return
	}
	saved, err := s.o.Profiles.Put(p)
	if err != nil {
		fail(w, http.StatusInternalServerError, "profiles")
		return
	}
	s.mu.Lock()
	s.prof = saved
	s.mu.Unlock()
	s.changed()
	writeJSON(w, http.StatusOK, s.stateBody())
}

func (s *Server) use(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &req) {
		return
	}
	p, err := s.o.Profiles.Get(req.ID)
	if err != nil {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.open(r, p); err != nil {
		s.openError(w, err)
		return
	}
	s.changed()
	writeJSON(w, http.StatusOK, s.stateBody())
}

func (s *Server) disconnect(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	if s.sess != nil {
		s.sess.Close()
	}
	s.sess = nil
	s.mu.Unlock()
	s.changed()
	writeJSON(w, http.StatusOK, s.stateBody())
}

func (s *Server) forget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &req) {
		return
	}
	_ = s.o.Secrets.Delete(req.ID)
	_ = s.o.Secrets.Delete("session:" + req.ID)
	if err := s.o.Profiles.Delete(req.ID); err != nil {
		fail(w, http.StatusInternalServerError, "profiles")
		return
	}
	writeJSON(w, http.StatusOK, s.stateBody())
}

func (s *Server) rememberLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login string `json:"login"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.mu.Lock()
	id := s.prof.ID
	s.mu.Unlock()
	if id != "" {
		_ = s.o.Profiles.SetLogin(id, req.Login)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// A saved password is handed to the page only for the profile that is connected: the page fills the sign-in form with it.
func (s *Server) secretProfile(r *http.Request) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id := r.URL.Query().Get("profile"); id != "" && id == s.prof.ID {
		return id
	}
	return ""
}

// maxVault is the biggest vault file (the Keys): the file is read whole into the page.
const maxVault = 1 << 20

// vaultOpen asks for a vault file (a native dialog) and returns its bytes: the file of the Keys is the person's own,
// the page decrypts it. Only a file of a limited size with the extension of the vault is read.
func (s *Server) vaultOpen(w http.ResponseWriter, _ *http.Request) {
	if s.o.PickFile == nil {
		fail(w, http.StatusNotImplemented, "no_native_dialog")
		return
	}
	path, ok, err := s.o.PickFile(".lvault")
	if err != nil {
		fail(w, http.StatusInternalServerError, "dialog")
		return
	}
	if !ok || path == "" {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		fail(w, http.StatusBadRequest, "not_a_file")
		return
	}
	if st.Size() > maxVault {
		fail(w, http.StatusRequestEntityTooLarge, "too_big")
		return
	}
	b, err := os.ReadFile(path) //nolint:gosec // the person chose this path in the dialog
	if err != nil {
		fail(w, http.StatusInternalServerError, "read")
		return
	}
	s.vault.remember(path)
	writeJSON(w, http.StatusOK, map[string]any{"name": filepath.Base(path), "data": base64.StdEncoding.EncodeToString(b), "sum": sumOf(b)})
}

// vaultLast returns the vault file chosen earlier (within seven days) so that the page need not ask for it again.
func (s *Server) vaultLast(w http.ResponseWriter, _ *http.Request) {
	path, ok := s.vault.recall()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxVault {
		s.vault.forget() // moved, deleted or not a vault any more
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	b, err := os.ReadFile(path) //nolint:gosec // the path is the one the person chose in the dialog
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": filepath.Base(path), "data": base64.StdEncoding.EncodeToString(b), "sum": sumOf(b)})
}

func (s *Server) vaultForget(w http.ResponseWriter, _ *http.Request) {
	s.vault.forget()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// vaultSave asks where to save the vault file (a native dialog) and writes the bytes the page sends (already encrypted).
func (s *Server) vaultSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	if !decode(w, r, &req) {
		return
	}
	if s.o.SaveFile == nil {
		fail(w, http.StatusNotImplemented, "no_native_dialog")
		return
	}
	b, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil || len(b) == 0 || len(b) > maxVault {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	name := filepath.Base(strings.ReplaceAll(req.Name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "linager-keys.lvault"
	}
	dest, ok, err := s.o.SaveFile(name)
	if err != nil {
		fail(w, http.StatusInternalServerError, "dialog")
		return
	}
	if !ok || dest == "" {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	// the file holds passwords (encrypted): readable by its owner only
	if err := os.WriteFile(dest, b, 0o600); err != nil {
		fail(w, http.StatusInternalServerError, "write")
		return
	}
	s.vault.remember(dest)
	writeJSON(w, http.StatusOK, map[string]any{"saved": dest, "name": filepath.Base(dest), "sum": sumOf(b)})
}

// vaultWrite writes the vault into the file that is open (the one remembered by the application), with no dialog: the Keys save
// every change by themselves. `base` is the fingerprint of the file as the page last saw it; when the file on the disk is another
// (changed by a sync tool, or another file was opened in another window) it answers 409 "changed" and writes nothing, unless
// `force` says to overwrite. 404 "no_file" when no file is remembered, 404 "gone" when it was deleted: the page then asks where to save.
func (s *Server) vaultWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data  string `json:"data"`
		Base  string `json:"base"`
		Force bool   `json:"force"`
	}
	if !decode(w, r, &req) {
		return
	}
	b, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil || len(b) == 0 || len(b) > maxVault {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	path, ok := s.vault.recall()
	if !ok {
		fail(w, http.StatusNotFound, "no_file")
		return
	}
	cur, err := os.ReadFile(path) //nolint:gosec // the path is the one the person chose in the dialog
	if err != nil {
		if os.IsNotExist(err) {
			s.vault.forget()
			fail(w, http.StatusNotFound, "gone")
			return
		}
		fail(w, http.StatusInternalServerError, "read")
		return
	}
	if !req.Force && req.Base != "" && sumOf(cur) != req.Base {
		fail(w, http.StatusConflict, "changed")
		return
	}
	if err := writeVaultInPlace(path, cur, b); err != nil {
		fail(w, http.StatusInternalServerError, "write")
		return
	}
	s.vault.remember(path) // writing is a use: the seven days start again
	writeJSON(w, http.StatusOK, map[string]any{"name": filepath.Base(path), "sum": sumOf(b)})
}

// downloadPath is what a download may fetch: the file or the zip of the file manager of a server, or the CSV of its metrics.
var downloadPath = regexp.MustCompile(`^api/servers/[A-Za-z0-9-]+/(files/(download|zip)|metrics\.csv)\?`)

// download saves a file of the server to the disk: the web view of the application does not handle a download by
// itself, so the application asks where (a native dialog) and writes the file.
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	if s.o.SaveFile == nil {
		fail(w, http.StatusNotImplemented, "no_native_dialog")
		return
	}
	if !downloadPath.MatchString(req.Path) {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	s.mu.Lock()
	sess := s.sess
	s.mu.Unlock()
	if sess == nil {
		fail(w, http.StatusServiceUnavailable, "not_connected")
		return
	}
	name := filepath.Base(strings.ReplaceAll(req.Name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "download"
	}
	dest, ok, err := s.o.SaveFile(name)
	if err != nil {
		fail(w, http.StatusInternalServerError, "dialog")
		return
	}
	if !ok || dest == "" {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	resp, err := sess.Fetch(r.Context(), "/"+req.Path)
	if err != nil {
		fail(w, http.StatusBadGateway, "upstream")
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fail(w, resp.StatusCode, "upstream")
		return
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // the person chose this path in the dialog
	if err != nil {
		fail(w, http.StatusInternalServerError, "write")
		return
	}
	_, cerr := io.Copy(f, resp.Body)
	if e := f.Close(); cerr == nil {
		cerr = e
	}
	if cerr != nil {
		_ = os.Remove(dest)
		fail(w, http.StatusBadGateway, "interrupted")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": dest})
}

func (s *Server) prefsGet(w http.ResponseWriter, _ *http.Request) {
	if s.o.Prefs == nil {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	writeJSON(w, http.StatusOK, s.o.Prefs.All())
}

func (s *Server) prefsPut(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if !decode(w, r, &in) {
		return
	}
	if s.o.Prefs == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if err := s.o.Prefs.Merge(in); err != nil {
		fail(w, http.StatusBadRequest, "prefs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// trayGet lists the connections of the tray for the profile that is open.
func (s *Server) trayGet(w http.ResponseWriter, _ *http.Request) {
	list := []tray.Entry{}
	if p := s.Profile(); p != "" && s.o.Tray != nil {
		list = s.o.Tray.List(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": list, "native": s.o.OnTray != nil})
}

// trayPut puts the connections of a server in the tray (an empty list takes it out).
func (s *Server) trayPut(w http.ResponseWriter, r *http.Request) {
	var e tray.Entry
	if !decode(w, r, &e) {
		return
	}
	p := s.Profile()
	if p == "" || s.o.Tray == nil {
		fail(w, http.StatusConflict, "not_connected")
		return
	}
	if err := s.o.Tray.Set(p, e); err != nil {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	s.changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) fullscreenGet(w http.ResponseWriter, _ *http.Request) {
	if s.o.Fullscreen == nil {
		fail(w, http.StatusNotImplemented, "no_native_windows")
		return
	}
	on, err := s.o.Fullscreen(nil)
	if err != nil {
		fail(w, http.StatusInternalServerError, "window")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"on": on})
}

func (s *Server) fullscreenPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if !decode(w, r, &req) {
		return
	}
	if s.o.Fullscreen == nil {
		fail(w, http.StatusNotImplemented, "no_native_windows")
		return
	}
	on, err := s.o.Fullscreen(&req.On)
	if err != nil {
		fail(w, http.StatusInternalServerError, "window")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"on": on})
}

// maxFitDelta bounds what the page may ask: a window is never made bigger than any screen by it.
const maxFitDelta = 10000

func (s *Server) windowFit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DW int `json:"dw"`
		DH int `json:"dh"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.DW < -maxFitDelta || req.DW > maxFitDelta || req.DH < -maxFitDelta || req.DH > maxFitDelta {
		fail(w, http.StatusBadRequest, "bad_size")
		return
	}
	if s.o.FitWindow == nil {
		fail(w, http.StatusNotImplemented, "no_native_windows")
		return
	}
	if err := s.o.FitWindow(req.DW, req.DH); err != nil {
		fail(w, http.StatusInternalServerError, "window")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) autostartGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"supported": s.o.AutostartExe != "", "enabled": s.o.AutostartExe != "" && autostart.Enabled()})
}

func (s *Server) autostartPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &req) {
		return
	}
	if s.o.AutostartExe == "" {
		fail(w, http.StatusNotImplemented, "not_supported")
		return
	}
	if err := autostart.Set(req.Enabled, s.o.AutostartExe); err != nil {
		fail(w, http.StatusInternalServerError, "autostart")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"supported": true, "enabled": autostart.Enabled()})
}

// updateInfo answers what the last check found; without an updater the build cannot update itself.
func (s *Server) updateInfo(w http.ResponseWriter, r *http.Request) {
	if s.o.Update == nil {
		ch := update.ChannelOf(s.o.Version)
		if c := r.URL.Query().Get("channel"); c == "stable" || c == "beta" {
			ch = c
		}
		writeJSON(w, http.StatusOK, update.Info{Reason: "headless", Current: s.o.Version, Channel: ch})
		return
	}
	if r.URL.Query().Has("channel") {
		s.o.Update.SetChannel(r.URL.Query().Get("channel"))
	}
	writeJSON(w, http.StatusOK, s.o.Update.Info())
}

func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	if s.o.Update == nil {
		s.updateInfo(w, r)
		return
	}
	// the channel the person chose is sent with the check ("stable" or "beta"; anything else follows the running version)
	s.o.Update.SetChannel(r.URL.Query().Get("channel"))
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	info, err := s.o.Update.Check(ctx)
	if err != nil {
		body := map[string]any{"error": "update_check", "info": info, "problem": "network"}
		// GitHub lets an address ask 60 times an hour; when it refuses for that, say when it may be asked again
		var se *update.StatusError
		if errors.As(err, &se) {
			body["problem"] = "other"
			if se.RateLimited() {
				body["problem"] = "rate_limited"
				if !se.Reset.IsZero() {
					body["retry_at"] = se.Reset.Unix()
				}
			}
		}
		writeJSON(w, http.StatusBadGateway, body)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// githubTokenKey is where the GitHub token of the update checks lives in the keychain (it never goes to the page).
const githubTokenKey = "github-token"

func (s *Server) hasToken() bool {
	v, err := s.o.Secrets.Get(githubTokenKey)
	return err == nil && v != ""
}

func (s *Server) tokenGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"set": s.hasToken(), "supported": s.o.Update != nil})
}

// tokenPut checks a GitHub token with GitHub and keeps it in the keychain. The checks of updates then count against its limit.
func (s *Server) tokenPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		return
	}
	if s.o.Update == nil {
		fail(w, http.StatusNotImplemented, "not_supported")
		return
	}
	tok := strings.TrimSpace(req.Token)
	if tok == "" || len(tok) > 255 || strings.ContainsAny(tok, " \t\r\n") {
		fail(w, http.StatusBadRequest, "bad_token")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := s.o.Update.VerifyToken(ctx, tok); err != nil {
		var se *update.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			fail(w, http.StatusUnprocessableEntity, "invalid_token")
			return
		}
		fail(w, http.StatusBadGateway, "network")
		return
	}
	if err := s.o.Secrets.Set(githubTokenKey, tok); err != nil {
		fail(w, http.StatusInternalServerError, "keychain")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"set": true})
}

func (s *Server) tokenDelete(w http.ResponseWriter, _ *http.Request) {
	_ = s.o.Secrets.Delete(githubTokenKey)
	writeJSON(w, http.StatusOK, map[string]bool{"set": false})
}

// updateInstall starts the installation and answers at once; the page follows it through /app/update/status.
func (s *Server) updateInstall(w http.ResponseWriter, _ *http.Request) {
	if s.o.Update == nil {
		fail(w, http.StatusNotImplemented, "not_supported")
		return
	}
	if st := s.o.Update.Status(); st.Phase == "downloading" || st.Phase == "installing" || st.Phase == "restarting" {
		writeJSON(w, http.StatusAccepted, st)
		return
	}
	go func() {
		// the request is over by then, so the installation has a context of its own
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		_ = s.o.Update.Install(ctx)
	}()
	writeJSON(w, http.StatusAccepted, update.Status{Phase: "downloading"})
}

func (s *Server) updateStatus(w http.ResponseWriter, _ *http.Request) {
	if s.o.Update == nil {
		writeJSON(w, http.StatusOK, update.Status{Phase: "idle"})
		return
	}
	st := s.o.Update.Status()
	if st.Phase == "" {
		st.Phase = "idle"
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) secretGet(w http.ResponseWriter, r *http.Request) {
	id := s.secretProfile(r)
	if id == "" {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	pw, err := s.o.Secrets.Get(id)
	if err != nil {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": pw})
}

func (s *Server) secretPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile  string `json:"profile"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.mu.Lock()
	ok := req.Profile != "" && req.Profile == s.prof.ID && req.Password != ""
	s.mu.Unlock()
	if !ok {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.o.Secrets.Set(req.Profile, req.Password); err != nil {
		fail(w, http.StatusInternalServerError, "keychain")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) secretDelete(w http.ResponseWriter, r *http.Request) {
	if id := s.secretProfile(r); id != "" {
		_ = s.o.Secrets.Delete(id)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) window(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Title string `json:"title"`
		Arg   string `json:"arg"` // a path for the editor
	}
	if !decode(w, r, &req) {
		return
	}
	if s.o.OpenWindow == nil {
		fail(w, http.StatusNotImplemented, "no_native_windows")
		return
	}
	if !windowKinds[req.Kind] || req.ID == "" {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.o.OpenWindow(req.Kind, req.ID, req.Title, req.Arg); err != nil {
		fail(w, http.StatusInternalServerError, "window")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sess := s.sess
	s.mu.Unlock()
	if sess == nil {
		fail(w, http.StatusServiceUnavailable, "not_connected")
		return
	}
	rec := &statusWriter{ResponseWriter: w}
	sess.Handler().ServeHTTP(rec, r)
	// the server no longer knows the kept session (it was idle for seven days, or it was ended): forget it
	if rec.status == http.StatusUnauthorized && r.URL.Path == "/api/me" {
		s.mu.Lock()
		cur := s.prof
		s.mu.Unlock()
		s.keepSession(cur, "")
	}
}

// statusWriter remembers the status of an answer; Flush and Hijack still reach the connection (the live stream and
// the terminals need them).
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// windowKinds are the windows the interface may ask for.
var windowKinds = map[string]bool{"terminal": true, "note": true, "files": true, "manager": true, "editor": true, "rdp": true, "asst": true, "settings": true}
