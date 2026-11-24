package server_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bolgov0zero/linager/agent"
	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

type connStatusView struct {
	AgentPort   int    `json:"agent_port"`
	AppPort     int    `json:"app_port"`
	AppUp       bool   `json:"app_up"`
	Fingerprint string `json:"fingerprint"`
	MinAgent    string `json:"min_agent"`
	Move        *struct {
		From    int `json:"from"`
		To      int `json:"to"`
		Moved   int `json:"moved"`
		Total   int `json:"total"`
		Pending []struct {
			Name      string `json:"name"`
			Supported bool   `json:"supported"`
		} `json:"pending"`
	} `json:"move"`
	Blockers []struct {
		Name string `json:"name"`
	} `json:"blockers"`
}

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		_ = c.Close()
	}
	return err == nil
}

func portOf(addr string) int {
	_, p, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(p)
	return n
}

// TestConnectionPortsAndMove: the application port serves the API over TLS with the certificate of the agent port,
// both ports are changed in the panel without a restart, a change of the agent port is refused while an online
// agent cannot follow it, and an agent that was offline meanwhile finds the new port by itself.
func TestConnectionPortsAndMove(t *testing.T) {
	dsn := os.Getenv("LINAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("LINAGER_TEST_DSN is not set")
	}
	if !strings.Contains(dsn, "test") {
		t.Fatal("refusing to wipe a database whose DSN does not contain 'test'")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pc, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pc.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	_ = pc.Close(ctx)

	dir := t.TempDir()
	cfg := &config.Server{DataDir: dir}
	cfg.Database.DSN = dsn
	cfg.Admin.Listen, cfg.Agents.Listen, cfg.App.Listen = freeAddr(t), freeAddr(t), freeAddr(t)
	cfg.Agents.Hosts = []string{"127.0.0.1"}
	cfg.Admin.CookieSecure = "never"
	cfg.Admin.LoginRate = 1000
	if err := server.InitKeys(dir, cfg.Agents.Hosts); err != nil {
		t.Fatal(err)
	}
	keys, err := server.LoadKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	serverFP := pki.Fingerprint(keys.TLSDER)
	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := server.CreateAdmin(ctx, db, "admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx, cfg, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	defer func() { cancel(); <-done }()
	waitFor(t, "server", func() bool { return portOpen(cfg.Admin.Listen) && portOpen(cfg.App.Listen) })

	jar, _ := cookiejar.New(nil)
	api := &apiClient{t: t, base: "http://" + cfg.Admin.Listen, hc: &http.Client{Jar: jar}}
	var lr struct{ CSRF string }
	if code := api.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password"}, &lr); code != 200 {
		t.Fatalf("login: %d", code)
	}
	api.csrf = lr.CSRF

	// --- the state ---
	var st connStatusView
	if code := api.do("GET", "/api/connection", nil, &st); code != 200 || st.AgentPort != portOf(cfg.Agents.Listen) || st.AppPort != portOf(cfg.App.Listen) || !st.AppUp ||
		st.Fingerprint != serverFP || st.MinAgent == "" || st.Move != nil {
		t.Fatalf("connection state: %d %+v", code, st)
	}

	// --- the application port: the same API over TLS, with the certificate of the agent port ---
	appGet := func(addr string) (int, string) {
		hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, //nolint:gosec // the pin is checked below
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 || pki.Fingerprint(cs.PeerCertificates[0].Raw) != serverFP {
					return io.ErrUnexpectedEOF
				}
				return nil
			}}}, Timeout: 3 * time.Second}
		resp, err := hc.Get("https://" + addr + "/api/me")
		if err != nil {
			return 0, err.Error()
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := appGet(cfg.App.Listen); code != http.StatusUnauthorized {
		t.Fatalf("the application port must serve the API (401 without a session): %d %s", code, body)
	}
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": st.AgentPort, "app_port": st.AgentPort}, nil); code != http.StatusConflict {
		t.Fatalf("the same port for both listeners must be refused: %d", code)
	}
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": 70000, "app_port": 0}, nil); code != http.StatusBadRequest {
		t.Fatalf("a bad port must be refused: %d", code)
	}
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": st.AgentPort, "app_port": portOf(cfg.Admin.Listen)}, nil); code != http.StatusConflict {
		t.Fatalf("the port of the admin panel must be refused: %d", code)
	}
	busy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = busy.Close() }()
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": st.AgentPort, "app_port": busy.Addr().(*net.TCPAddr).Port}, nil); code != http.StatusConflict {
		t.Fatalf("a busy port must be refused: %d", code)
	}
	if !portOpen(cfg.App.Listen) {
		t.Fatal("a refused change must leave the old port open")
	}
	newApp := freeAddr(t)
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": st.AgentPort, "app_port": portOf(newApp)}, &st); code != 200 || st.AppPort != portOf(newApp) {
		t.Fatalf("change the application port: %d %+v", code, st)
	}
	if code, _ := appGet(newApp); code != http.StatusUnauthorized {
		t.Fatalf("the new application port must work: %d", code)
	}
	waitFor(t, "the old application port is closed", func() bool { return !portOpen(cfg.App.Listen) })
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": st.AgentPort, "app_port": 0}, &st); code != 200 || st.AppUp {
		t.Fatalf("switch the application port off: %d %+v", code, st)
	}
	waitFor(t, "the application port is off", func() bool { return !portOpen(newApp) })

	// --- an agent that cannot follow a change of the agent port blocks it while it is online ---
	adata := t.TempDir()
	shownCh := make(chan string, 1)
	enrollErr := make(chan error, 1)
	go func() {
		enrollErr <- agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: serverFP, DataDir: adata, AgentVersion: "0.0.1-test", PollEvery: 100 * time.Millisecond,
			Show: func(k, _, _ string) { shownCh <- k }})
	}()
	key := <-shownCh
	var pending []struct {
		ID string `json:"id"`
	}
	waitFor(t, "pending enrollment", func() bool { api.do("GET", "/api/enrollments", nil, &pending); return len(pending) == 1 })
	if code := api.do("POST", "/api/enrollments/"+pending[0].ID+"/approve", map[string]string{"key": strings.ToLower(key)}, nil); code != 200 {
		t.Fatalf("approve: %d", code)
	}
	if err := <-enrollErr; err != nil {
		t.Fatal(err)
	}
	runAgent := func(version string) (stop func()) {
		actx, acancel := context.WithCancel(ctx)
		exited := make(chan struct{})
		go func() {
			_ = agent.Run(actx, agent.RunOptions{DataDir: adata, AgentVersion: version, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PingEvery: 100 * time.Millisecond,
				SampleEvery: 200 * time.Millisecond, Collector: fakeCollector{boot: time.Now().Add(-time.Hour).Unix()}})
			close(exited)
		}()
		return func() { acancel(); <-exited }
	}
	online := func(want bool) {
		waitFor(t, "agent online="+strconv.FormatBool(want), func() bool {
			var l []struct {
				Online bool `json:"online"`
			}
			api.do("GET", "/api/servers", nil, &l)
			return len(l) == 1 && l[0].Online == want
		})
	}
	stop := runAgent("0.0.1-test")
	online(true)
	oldAgentPort := portOf(cfg.Agents.Listen)
	newAgent := freeAddr(t)
	var refused struct {
		Error  string `json:"error"`
		Agents []struct {
			Name string `json:"name"`
		} `json:"agents"`
	}
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": portOf(newAgent), "app_port": 0}, &refused); code != http.StatusConflict || refused.Error != "agents_outdated" || len(refused.Agents) != 1 {
		t.Fatalf("an online agent that cannot follow must block the change: %d %+v", code, refused)
	}
	if code := api.do("GET", "/api/connection", nil, &st); code != 200 || len(st.Blockers) != 1 {
		t.Fatalf("the blocker is listed: %d %+v", code, st)
	}

	// --- offline, the old agent no longer blocks; the move starts and waits for it ---
	stop()
	online(false)
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": portOf(newAgent), "app_port": 0}, &st); code != 200 || st.AgentPort != portOf(newAgent) ||
		st.Move == nil || st.Move.From != oldAgentPort || st.Move.To != portOf(newAgent) || st.Move.Total != 1 || st.Move.Moved != 0 || len(st.Move.Pending) != 1 || st.Move.Pending[0].Supported {
		t.Fatalf("change the agent port: %d %+v", code, st)
	}
	if !portOpen(cfg.Agents.Listen) || !portOpen(newAgent) {
		t.Fatal("both agent ports must be open during the move")
	}
	if code := api.do("PUT", "/api/connection", map[string]int{"agent_port": portOf(freeAddr(t)), "app_port": 0}, nil); code != http.StatusConflict {
		t.Fatalf("a second change while agents are moving must be refused: %d", code)
	}
	var ci struct {
		Port string `json:"port"`
	}
	if code := api.do("GET", "/api/connect", nil, &ci); code != 200 || ci.Port != strconv.Itoa(portOf(newAgent)) {
		t.Fatalf("the install dialog must show the new port: %d %+v", code, ci)
	}

	// --- the agent comes back (updated) to the old port, is told the new one and moves ---
	stop = runAgent("0.10.0-beta.1")
	defer stop()
	waitFor(t, "the agent state holds the new port", func() bool {
		b, err := os.ReadFile(filepath.Join(adata, "state.json")) //nolint:gosec // a path inside the test directory
		if err != nil {
			return false
		}
		var s struct {
			Server string `json:"server"`
			Next   string `json:"next"`
		}
		return json.Unmarshal(b, &s) == nil && s.Server == newAgent && s.Next == ""
	})
	online(true)
	waitFor(t, "the old agent port is closed once everyone moved", func() bool {
		return api.do("GET", "/api/connection", nil, &st) == 200 && st.Move == nil && !portOpen(cfg.Agents.Listen)
	})
	if !portOpen(newAgent) {
		t.Fatal("the new agent port must stay open")
	}

	// --- a new port that does not work is given up by the agent: it stays on the old one and ignores that port for a while ---
	stop()
	online(false)
	dead := portOf(freeAddr(t))
	b, _ := os.ReadFile(filepath.Join(adata, "state.json")) //nolint:gosec // a path inside the test directory
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	raw["next"] = net.JoinHostPort("127.0.0.1", strconv.Itoa(dead))
	b, _ = json.Marshal(raw)
	if err := os.WriteFile(filepath.Join(adata, "state.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	stopLast := runAgent("0.10.0-beta.1")
	defer stopLast()
	online(true)
	b, _ = os.ReadFile(filepath.Join(adata, "state.json")) //nolint:gosec // a path inside the test directory
	var back struct {
		Server  string `json:"server"`
		Next    string `json:"next"`
		BadPort int    `json:"bad_port"`
	}
	if err := json.Unmarshal(b, &back); err != nil || back.Server != newAgent || back.Next != "" || back.BadPort != dead {
		t.Fatalf("the agent must fall back and remember the bad port: %+v %v", back, err)
	}
}

// TestAppSessionLifetime: a session made on the application port lasts seven days and is renewed by every request (after
// seven idle days the password and 2FA are asked again); a browser session keeps its fixed 12 hours.
func TestAppSessionLifetime(t *testing.T) {
	dsn := os.Getenv("LINAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("LINAGER_TEST_DSN is not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pc, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pc.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	_ = pc.Close(ctx)
	dir := t.TempDir()
	cfg := &config.Server{DataDir: dir}
	cfg.Database.DSN = dsn
	cfg.Admin.Listen, cfg.Agents.Listen, cfg.App.Listen = freeAddr(t), freeAddr(t), freeAddr(t)
	cfg.Agents.Hosts = []string{"127.0.0.1"}
	cfg.Admin.CookieSecure = "never"
	cfg.Admin.LoginRate = 1000
	if err := server.InitKeys(dir, cfg.Agents.Hosts); err != nil {
		t.Fatal(err)
	}
	keys, _ := server.LoadKeys(dir)
	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := server.CreateAdmin(ctx, db, "admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx, cfg, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	defer func() { cancel(); <-done }()
	waitFor(t, "server", func() bool { return portOpen(cfg.Admin.Listen) && portOpen(cfg.App.Listen) })

	fp := pki.Fingerprint(keys.TLSDER)
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, //nolint:gosec // the pin is checked below
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 || pki.Fingerprint(cs.PeerCertificates[0].Raw) != fp {
				return io.ErrUnexpectedEOF
			}
			return nil
		}}}}
	app := &apiClient{t: t, base: "https://" + cfg.App.Listen, hc: hc}
	var lr struct{ CSRF string }
	if code := app.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password"}, &lr); code != 200 {
		t.Fatalf("sign-in on the application port: %d", code)
	}
	browser := &apiClient{t: t, base: "http://" + cfg.Admin.Listen, hc: &http.Client{Jar: func() *cookiejar.Jar { j, _ := cookiejar.New(nil); return j }()}}
	browser.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password"}, nil)

	type row struct {
		sliding int
		secs    float64
	}
	rows := func() []row {
		r, err := db.Pool.Query(ctx, `SELECT sliding_secs, extract(epoch from expires_at - now()) FROM sessions ORDER BY sliding_secs`)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		var out []row
		for r.Next() {
			var x row
			if err := r.Scan(&x.sliding, &x.secs); err != nil {
				t.Fatal(err)
			}
			out = append(out, x)
		}
		return out
	}
	got := rows()
	if len(got) != 2 || got[0].sliding != 0 || got[0].secs > 12*3600 || got[1].sliding != 7*24*3600 || got[1].secs < 7*24*3600-60 {
		t.Fatalf("a browser session of 12 hours and an application session of 7 days: %+v", got)
	}
	// idle for six days, then a request: the seven days start again
	if _, err := db.Pool.Exec(ctx, `UPDATE sessions SET expires_at = now() + interval '1 day' WHERE sliding_secs > 0`); err != nil {
		t.Fatal(err)
	}
	if code := app.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatalf("a request on the application port: %d", code)
	}
	if got := rows(); got[1].secs < 7*24*3600-60 {
		t.Fatalf("a request must renew the session: %+v", got)
	}
	// seven idle days: the session is over
	if _, err := db.Pool.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 second' WHERE sliding_secs > 0`); err != nil {
		t.Fatal(err)
	}
	if code := app.do("GET", "/api/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("an idle session must end: %d", code)
	}
	// the browser session does not slide
	before := rows()[0].secs
	browser.do("GET", "/api/me", nil, nil)
	if after := rows()[0].secs; after > before+1 {
		t.Fatalf("a browser session must not slide: %v -> %v", before, after)
	}
}
