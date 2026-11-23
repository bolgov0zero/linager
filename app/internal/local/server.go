// Package local is the server inside the application that the windows talk to. It listens on the loopback address
// only, every path starts with a random token (another program or web page that finds the port cannot use it),
// and it does three things: it serves the interface, it proxies the API of the Linager server over the pinned
// connection (internal/remote), and it offers the few native things a web page cannot do (profiles, the keychain,
// opening a window).
package local

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bolgov0zero/linager/app/internal/profiles"
	"github.com/bolgov0zero/linager/app/internal/remote"
)

// Options configure the server.
type Options struct {
	Assets   fs.FS // the built interface (index.html at the root)
	Profiles *profiles.Store
	Secrets  profiles.Secrets
	// Debug logs every request (without the token), for development.
	Debug bool
	// OpenWindow opens (or raises) a native window of a kind ("terminal") for a server; nil in the headless run.
	OpenWindow func(kind, id, title string) error
	Version    string
}

// Server is the local server.
type Server struct {
	o     Options
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
	s := &Server{o: o, token: hex.EncodeToString(b), ln: ln}
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
	Saved       bool   `json:"saved"` // a password is in the keychain
}

func (s *Server) view(p profiles.Profile) profileView {
	_, err := s.o.Secrets.Get(p.ID)
	return profileView{ID: p.ID, Name: p.Name, Address: p.Address, Fingerprint: p.Fingerprint, Login: p.Login, Saved: err == nil}
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
	sess, err := remote.NewSession(remote.Target{Addr: p.Address, Fingerprint: p.Fingerprint, BasePath: p.BasePath})
	if err != nil {
		return err
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
	return nil
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
		BasePath    string `json:"base_path"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Address = strings.TrimSpace(req.Address)
	if req.Address == "" || !strings.HasPrefix(req.Fingerprint, "SHA256:") {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	p := profiles.Profile{Name: req.Name, Address: req.Address, Fingerprint: req.Fingerprint, BasePath: req.BasePath}
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
	writeJSON(w, http.StatusOK, s.stateBody())
}

func (s *Server) disconnect(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	if s.sess != nil {
		s.sess.Close()
	}
	s.sess = nil
	s.mu.Unlock()
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
	}
	if !decode(w, r, &req) {
		return
	}
	if s.o.OpenWindow == nil {
		fail(w, http.StatusNotImplemented, "no_native_windows")
		return
	}
	if req.Kind != "terminal" || req.ID == "" {
		fail(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := s.o.OpenWindow(req.Kind, req.ID, req.Title); err != nil {
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
	sess.Handler().ServeHTTP(w, r)
}
