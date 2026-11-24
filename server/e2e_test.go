package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pquerna/otp/totp"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"

	"github.com/bolgov0zero/linager/agent"
	"github.com/bolgov0zero/linager/internal/assist"
	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/internal/protocol"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

type fakeCollector struct{ boot int64 }

func (f fakeCollector) Collect(now time.Time) protocol.Sample {
	return protocol.Sample{TS: now.Unix(), CPU: 42, MemPct: 55, MemUsed: 55, MemTotal: 100, DiskPct: 33, RxBps: 1000, TxBps: 500,
		BootTime: f.boot, Uptime: now.Unix() - f.boot, Updates: 3,
		Disks: []protocol.Disk{{Mount: "/", Device: "/dev/x", FS: "ext4", Total: 100, Used: 33, Pct: 33}}}
}

func (fakeCollector) System(v string) protocol.SystemInfo {
	return protocol.SystemInfo{Hostname: "h", OS: "Test OS", Kernel: "6.0", Arch: "amd64", CPUModel: "cpu", CPUCores: 2, MemTotal: 100, AgentVersion: v}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String()
}

type apiClient struct {
	t    *testing.T
	base string
	hc   *http.Client
	csrf string
}

func (c *apiClient) do(method, path string, body any, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", what)
}

// TestEndToEnd needs a throw-away PostgreSQL database: set LINAGER_TEST_DSN (its name must contain "test").
func TestEndToEnd(t *testing.T) {
	dsn := os.Getenv("LINAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("LINAGER_TEST_DSN is not set")
	}
	if !strings.Contains(dsn, "test") {
		t.Fatal("refusing to wipe a database whose DSN does not contain 'test'")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)

	dir := t.TempDir()
	cfg := &config.Server{DataDir: dir}
	cfg.Database.DSN = dsn
	cfg.App.Listen, cfg.Agents.Listen = freeAddr(t), freeAddr(t)
	cfg.Agents.Hosts = []string{"127.0.0.1"}
	cfg.Admin.CookieSecure = "never"
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
	if err := server.CreateAdmin(ctx, db, "Admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	if err := server.CreateAdmin(ctx, db, "second", "another-long-password"); err == nil {
		t.Fatal("a second administrator must be refused in this version")
	}

	srvDone := make(chan error, 1)
	go func() { srvDone <- server.Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	waitFor(t, "server listening", func() bool {
		c, err := net.Dial("tcp", cfg.App.Listen)
		if err == nil {
			_ = c.Close()
		}
		return err == nil
	})

	jar, _ := cookiejar.New(nil)
	api := &apiClient{t: t, base: "https://" + cfg.App.Listen, hc: testClient(jar)}

	// --- authentication ---
	if code := api.do("GET", "/api/servers", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request: %d", code)
	}
	if code := api.do("POST", "/api/login", map[string]string{"login": "admin", "password": "wrong-password-1"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("bad password: %d", code)
	}
	var lr struct{ CSRF string }
	if code := api.do("POST", "/api/login", map[string]string{"login": "ADMIN", "password": "a-long-test-password"}, &lr); code != 200 || lr.CSRF == "" {
		t.Fatalf("login: %d", code)
	}
	api.csrf = ""
	if code := api.do("DELETE", "/api/servers/x", map[string]any{}, nil); code != http.StatusForbidden {
		t.Fatalf("missing CSRF must be refused, got %d", code)
	}
	api.csrf = lr.CSRF

	// --- connection info for clients (shown in the "Add server" dialog) ---
	var ci struct {
		Port        string   `json:"port"`
		Fingerprint string   `json:"fingerprint"`
		Hosts       []string `json:"hosts"`
	}
	if code := api.do("GET", "/api/connect", nil, &ci); code != 200 || ci.Fingerprint != serverFP || ci.Port != strings.Split(cfg.Agents.Listen, ":")[1] || len(ci.Hosts) != 1 {
		t.Fatalf("connect info: %d %+v", code, ci)
	}

	// --- server groups: create, duplicate name refused, rename, list, delete ---
	var grp struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if code := api.do("POST", "/api/groups", map[string]string{"name": " Prod "}, &grp); code != 200 || grp.Name != "Prod" {
		t.Fatalf("group create: %d %+v", code, grp)
	}
	if code := api.do("POST", "/api/groups", map[string]string{"name": "prod"}, nil); code != http.StatusConflict {
		t.Fatalf("duplicate group name must be refused, got %d", code)
	}
	if code := api.do("PUT", "/api/groups/"+strconv.FormatInt(grp.ID, 10), map[string]string{"name": "Production"}, nil); code != 200 {
		t.Fatalf("group rename: %d", code)
	}
	var gl []struct{ Name string }
	if code := api.do("GET", "/api/groups", nil, &gl); code != 200 || len(gl) != 1 || gl[0].Name != "Production" {
		t.Fatalf("group list: %d %+v", code, gl)
	}
	// the order: a new group goes last, an arranged order is kept, a wrong list is refused
	var grp2 struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if code := api.do("POST", "/api/groups", map[string]string{"name": "Test"}, &grp2); code != 200 {
		t.Fatalf("second group create: %d", code)
	}
	var ordered []struct{ Name string }
	if code := api.do("GET", "/api/groups", nil, &ordered); code != 200 || len(ordered) != 2 || ordered[1].Name != "Test" {
		t.Fatalf("a new group must go last: %d %+v", code, ordered)
	}
	// an arranged order is kept: "Test" goes before "Production"; a wrong place is refused
	gid := func(id int64) string { return strconv.FormatInt(id, 10) }
	if code := api.do("PUT", "/api/groups/"+gid(grp2.ID)+"/move", map[string]any{"parent_id": nil, "before_id": grp.ID}, nil); code != 200 {
		t.Fatalf("group move: %d", code)
	}
	if code := api.do("GET", "/api/groups", nil, &ordered); code != 200 || ordered[0].Name != "Test" || ordered[1].Name != "Production" {
		t.Fatalf("group order was not kept: %d %+v", code, ordered)
	}
	if code := api.do("PUT", "/api/groups/"+gid(grp2.ID)+"/move", map[string]any{"parent_id": nil, "before_id": 99999}, nil); code != http.StatusBadRequest {
		t.Fatalf("a place before an unknown folder must be refused, got %d", code)
	}
	for _, id := range []int64{grp.ID, grp2.ID} {
		if code := api.do("DELETE", "/api/groups/"+strconv.FormatInt(id, 10), nil, nil); code != 200 {
			t.Fatalf("group delete: %d", code)
		}
	}

	// --- enrollment with a wrong pinned fingerprint is refused ---
	adata := t.TempDir()
	err = agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: "SHA256:wrong", DataDir: adata, Show: func(string, string, string) {}})
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("fingerprint mismatch must abort, got %v", err)
	}

	// --- full pairing ---
	type shown struct{ key, code string }
	shownCh := make(chan shown, 1)
	enrollErr := make(chan error, 1)
	go func() {
		enrollErr <- agent.Enroll(ctx, agent.EnrollOptions{
			Server: cfg.Agents.Listen, Fingerprint: serverFP, DataDir: adata, AgentVersion: "0.0.1-test", PollEvery: 100 * time.Millisecond,
			Show: func(k, c, _ string) { shownCh <- shown{k, c} },
		})
	}()
	sh := <-shownCh
	type pend struct {
		ID         string `json:"id"`
		VerifyCode string `json:"verify_code"`
		Hostname   string `json:"hostname"`
	}
	var pending []pend
	waitFor(t, "pending enrollment", func() bool {
		api.do("GET", "/api/enrollments", nil, &pending)
		return len(pending) == 1
	})
	if pending[0].VerifyCode != sh.code {
		t.Fatalf("check codes differ: admin %q agent %q", pending[0].VerifyCode, sh.code)
	}
	if code := api.do("POST", "/api/enrollments/"+pending[0].ID+"/approve", map[string]string{"key": "ZZZZ-ZZZZ"}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong key: %d", code)
	}
	if code := api.do("POST", "/api/enrollments/"+pending[0].ID+"/approve", map[string]string{"key": strings.ToLower(sh.key)}, nil); code != 200 {
		t.Fatalf("approve: %d", code)
	}
	if err := <-enrollErr; err != nil {
		t.Fatalf("enroll: %v", err)
	}

	// --- signed connection ---
	var uninstalled struct {
		purge bool
		ran   bool
	}
	runErr := make(chan error, 1)
	go func() {
		runErr <- agent.Run(ctx, agent.RunOptions{
			DataDir: adata, AgentVersion: "0.0.1-test", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), PingEvery: 100 * time.Millisecond,
			SampleEvery: 200 * time.Millisecond, Collector: fakeCollector{boot: time.Now().Add(-time.Hour).Unix()},
			Uninstall:    func(purge bool, _ string) error { uninstalled.purge, uninstalled.ran = purge, true; return nil },
			ExtraStreams: map[string]agent.StreamFunc{"assistant": agent.FakeAssistStream()},
		})
	}()
	type srvView struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		AgentVersion string `json:"agent_version"`
		Online       bool   `json:"online"`
	}
	var servers []srvView
	waitFor(t, "agent online", func() bool {
		api.do("GET", "/api/servers", nil, &servers)
		return len(servers) == 1 && servers[0].Online
	})
	id := servers[0].ID
	if servers[0].AgentVersion != "0.0.1-test" {
		t.Fatalf("agent version not recorded: %+v", servers[0])
	}

	// --- metrics, dashboard, history ---
	type dashItem struct {
		ID     string           `json:"id"`
		Online bool             `json:"online"`
		Sample *protocol.Sample `json:"sample"`
		Spark  []float64        `json:"spark"`
	}
	var dash struct {
		Items []dashItem `json:"items"`
	}
	waitFor(t, "live sample on the dashboard", func() bool {
		api.do("GET", "/api/dashboard", nil, &dash)
		return len(dash.Items) == 1 && dash.Items[0].Sample != nil && len(dash.Items[0].Spark) > 0
	})
	if sm := dash.Items[0].Sample; sm.CPU != 42 || sm.Updates != 3 || len(sm.Disks) != 1 {
		t.Fatalf("unexpected sample: %+v", sm)
	}
	var hist struct {
		Tier   string                           `json:"tier"`
		Points []map[string]any                 `json:"points"`
		Stats  map[string]struct{ Avg float64 } `json:"stats"`
		Events []struct{ Kind string }          `json:"events"`
	}
	waitFor(t, "raw history", func() bool {
		api.do("GET", "/api/servers/"+id+"/metrics?period=1h", nil, &hist)
		return len(hist.Points) > 0
	})
	if hist.Tier != "raw" || hist.Stats["cpu"].Avg != 42 {
		t.Fatalf("raw history: tier=%s stats=%+v", hist.Tier, hist.Stats)
	}
	// the server page seeds its overview bars from the stored samples
	var seeded struct {
		Recent []struct {
			TS  int64   `json:"ts"`
			CPU float64 `json:"cpu"`
		} `json:"recent"`
	}
	api.do("GET", "/api/servers/"+id, nil, &seeded)
	if len(seeded.Recent) == 0 || seeded.Recent[len(seeded.Recent)-1].CPU != 42 {
		t.Fatalf("server detail must carry recent samples: %+v", seeded.Recent)
	}
	(&server.Scheduler{DB: db, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).RunOnce(ctx)
	for _, p := range []string{"24h", "7d", "30d", "90d", "180d", "360d"} {
		hist.Points = nil
		api.do("GET", "/api/servers/"+id+"/metrics?period="+p, nil, &hist)
		if len(hist.Points) == 0 || hist.Stats["cpu"].Avg != 42 {
			t.Fatalf("period %s (tier %s): points=%d stats=%+v", p, hist.Tier, len(hist.Points), hist.Stats)
		}
	}
	if code := api.do("GET", "/api/servers/"+id+"/metrics?period=2y", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown period must be refused, got %d", code)
	}
	hasOnline := false
	for _, e := range hist.Events {
		hasOnline = hasOnline || e.Kind == "online"
	}
	if !hasOnline {
		t.Fatal("an 'online' event must be recorded when the agent connects")
	}
	req, _ := http.NewRequest("GET", api.base+"/api/servers/"+id+"/metrics.csv?period=1h", nil)
	if resp, err := api.hc.Do(req); err != nil || resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") {
		t.Fatalf("csv export failed: %v", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if !strings.HasPrefix(string(body), "time_utc,cpu_avg") || strings.Count(string(body), "\n") < 2 {
			t.Fatalf("csv content: %q", body)
		}
	}
	// live stream delivers metric events
	sreq, _ := http.NewRequest("GET", api.base+"/api/live", nil)
	sctx, scancel := context.WithTimeout(ctx, 5*time.Second)
	sresp, err := api.hc.Do(sreq.WithContext(sctx))
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	got := ""
	for !strings.Contains(got, "event: metrics") {
		n, err := sresp.Body.Read(buf)
		got += string(buf[:n])
		if err != nil {
			scancel()
			t.Fatalf("live stream ended without a metrics event: %q (%v)", got, err)
		}
	}
	scancel()
	_ = sresp.Body.Close()
	// dashboard membership
	var avail []map[string]any
	if code := api.do("DELETE", "/api/dashboard/items/"+id, nil, nil); code != 200 {
		t.Fatalf("remove from dashboard: %d", code)
	}
	api.do("GET", "/api/dashboard/available", nil, &avail)
	api.do("GET", "/api/dashboard", nil, &dash)
	if len(avail) != 1 || len(dash.Items) != 0 {
		t.Fatalf("after remove: available=%d items=%d", len(avail), len(dash.Items))
	}
	if code := api.do("POST", "/api/dashboard/items", map[string]string{"server_id": id}, nil); code != 200 {
		t.Fatalf("add to dashboard: %d", code)
	}
	if code := api.do("PUT", "/api/dashboard/order", map[string]any{"server_ids": []string{id}}, nil); code != 200 {
		t.Fatalf("order: %d", code)
	}

	// --- management: confirmation rules, processes, streams ---
	if code := api.do("POST", "/api/servers/"+id+"/services/ssh.service/stop", map[string]any{"confirm": false}, nil); code != http.StatusConflict {
		t.Fatalf("stopping ssh without confirmation must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/services/linager-agent.service/restart", map[string]any{}, nil); code != http.StatusConflict {
		t.Fatalf("restarting the agent without confirmation must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/reboot", map[string]any{"confirm": false}, nil); code != http.StatusConflict {
		t.Fatalf("reboot without confirmation must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/services/bad;name/start", map[string]any{}, nil); code != http.StatusBadRequest {
		t.Fatalf("invalid unit names must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/processes/1/kill", map[string]any{"confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("killing pid 1 must be refused, got %d", code)
	}
	if code := api.do("GET", "/api/servers/"+id+"/processes?sort=cpu&n=5", nil, nil); code != http.StatusOK {
		t.Fatalf("processes list: %d", code)
	}
	// ending SSH sessions: validated and confirmed on the server, decided by the agent, audited either way
	endURL := "/api/servers/" + id + "/security/terminate"
	if code := api.do("POST", endURL, map[string]any{"session": "3;x", "confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("an invalid session id must be refused, got %d", code)
	}
	if code := api.do("POST", endURL, map[string]any{"session": "3", "user": "root", "confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("a session and a user together must be refused, got %d", code)
	}
	if code := api.do("POST", endURL, map[string]any{"session": "3"}, nil); code != http.StatusConflict {
		t.Fatalf("ending a session needs the confirmation, got %d", code)
	}
	// with the confirmation it reaches the agent; a test host has no such remote session, so the agent refuses
	if code := api.do("POST", endURL, map[string]any{"session": "3", "confirm": true}, nil); code == http.StatusOK {
		t.Fatalf("a session that does not exist cannot be ended, got %d", code)
	}
	var nTerm int
	_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action='security.session.terminate' AND result='failed'`).Scan(&nTerm)
	if nTerm != 1 {
		t.Fatalf("the refused attempt must be in the audit log, found %d", nTerm)
	}
	// fail2ban and authorized_keys: validated and confirmed on the server, decided by the agent
	banURL := "/api/servers/" + id + "/security/ban"
	if code := api.do("POST", banURL, map[string]any{"ip": "1.2.3.4; reboot", "confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("an invalid address must be refused, got %d", code)
	}
	if code := api.do("POST", banURL, map[string]any{"ip": "203.0.113.9"}, nil); code != http.StatusConflict {
		t.Fatalf("a ban needs the confirmation, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/security/unban", map[string]any{"confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("an unban without addresses must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/security/keys/remove", map[string]any{"user": "root", "file": "/root/.ssh/authorized_keys", "fingerprint": "SHA256:x", "confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("a malformed fingerprint must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/security/keys/remove", map[string]any{"user": "root", "file": "/root/.ssh/authorized_keys", "fingerprint": "SHA256:" + strings.Repeat("A", 43)}, nil); code != http.StatusConflict {
		t.Fatalf("removing a key needs the confirmation, got %d", code)
	}
	var detail struct {
		Modules []string `json:"modules"`
	}
	api.do("GET", "/api/servers/"+id, nil, &detail)
	if len(detail.Modules) < 5 {
		t.Fatalf("agent must report its modules: %v", detail.Modules)
	}
	if code := api.do("GET", "/api/servers/"+id+"/stream?kind=bogus", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown stream kind: %d", code)
	}
	wsURL := "ws" + strings.TrimPrefix(api.base, "http") + "/api/servers/" + id + "/stream?kind=terminal&cols=80&rows=24"
	wctx, wcancel := context.WithTimeout(ctx, 20*time.Second)
	wc, _, err := websocket.Dial(wctx, wsURL, &websocket.DialOptions{HTTPClient: api.hc})
	if err != nil {
		t.Fatalf("terminal websocket: %v", err)
	}
	if err := wc.Write(wctx, websocket.MessageBinary, []byte("echo hello-$((40+2))-linager\n")); err != nil {
		t.Fatal(err)
	}
	var term strings.Builder
	for !strings.Contains(term.String(), "hello-42-linager\r\n") {
		_, b, err := wc.Read(wctx)
		if err != nil {
			t.Fatalf("terminal output not received (%v): %q", err, term.String())
		}
		term.Write(b)
	}
	// The per-server limit (3) is reported as close code 4029, which browsers can read.
	var extra []*websocket.Conn
	for range 2 {
		c, _, err := websocket.Dial(wctx, wsURL, &websocket.DialOptions{HTTPClient: api.hc})
		if err != nil {
			t.Fatalf("terminal websocket: %v", err)
		}
		extra = append(extra, c)
	}
	over, _, err := websocket.Dial(wctx, wsURL, &websocket.DialOptions{HTTPClient: api.hc})
	if err != nil {
		t.Fatalf("over-limit terminal websocket: %v", err)
	}
	if _, _, err := over.Read(wctx); websocket.CloseStatus(err) != 4029 {
		t.Fatalf("over-limit terminal: want close 4029, got %v", err)
	}
	for _, c := range extra {
		_ = c.Close(websocket.StatusNormalClosure, "")
	}
	_ = wc.Close(websocket.StatusNormalClosure, "")
	wcancel()
	var nAudit int
	waitFor(t, "terminal audit entries", func() bool {
		_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action IN ('stream.open','stream.close')`).Scan(&nAudit)
		return nAudit >= 2
	})

	// --- the Assistant: pictures come to the browser, input goes back, the settings are kept and reach open windows ---
	var aset assist.Settings
	if code := api.do("GET", "/api/servers/"+id+"/assistant/settings", nil, &aset); code != 200 || aset != assist.DefaultSettings() {
		t.Fatalf("default assistant settings: %d %+v", code, aset)
	}
	for _, bad := range []assist.Settings{{Colors: "x", FPS: 10, Mbit: 2}, {Colors: "24", FPS: 99, Mbit: 2}, {Colors: "24", FPS: 10, Mbit: 99}} {
		if code := api.do("PUT", "/api/servers/"+id+"/assistant/settings", bad, nil); code != http.StatusUnprocessableEntity {
			t.Fatalf("bad assistant settings %+v: %d", bad, code)
		}
	}
	gray := assist.Settings{Colors: assist.ColorsGray, FPS: 10, Mbit: 2, Auto: true}
	if code := api.do("PUT", "/api/servers/"+id+"/assistant/settings", gray, nil); code != 200 {
		t.Fatalf("save assistant settings: %d", code)
	}
	if code := api.do("GET", "/api/servers/"+id+"/assistant/settings", nil, &aset); code != 200 || aset != gray {
		t.Fatalf("assistant settings are kept: %d %+v", code, aset)
	}
	if code := api.do("GET", "/api/servers/"+id+"/assistant?session=abc", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("a bad session: %d", code)
	}
	// a Linux agent lists the graphical sessions of its computer (none on a server), another host has no Assistant
	wantSessions := http.StatusNotImplemented
	if runtime.GOOS == "linux" {
		wantSessions = http.StatusOK
	}
	if code := api.do("GET", "/api/servers/"+id+"/assistant/sessions", nil, nil); code != wantSessions {
		t.Fatalf("the sessions of the Assistant: want %d, got %d", wantSessions, code)
	}
	actx, acancel := context.WithTimeout(ctx, 20*time.Second)
	aurl := "ws" + strings.TrimPrefix(api.base, "http") + "/api/servers/" + id + "/assistant?session=console"
	ac, _, err := websocket.Dial(actx, aurl, &websocket.DialOptions{HTTPClient: api.hc})
	if err != nil {
		t.Fatalf("assistant websocket: %v", err)
	}
	var gotHello, gotMonitor bool
	fullPictures := 0
	readAssist := func(until func() bool) {
		for !until() {
			_, b, err := ac.Read(actx)
			if err != nil {
				t.Fatalf("assistant frames: %v (hello %v, full pictures %d)", err, gotHello, fullPictures)
			}
			switch b[0] {
			case assist.TypeHello:
				gotHello = bytes.Contains(b[1:], []byte(`"w":960`))
			case assist.TypeTiles:
				if tl, err := assist.UnpackTiles(b[1:]); err == nil && len(tl) >= 8 {
					fullPictures++
				}
			}
		}
	}
	readAssist(func() bool { return gotHello && fullPictures >= 1 })
	// a second window on the same session is refused with a code the browser can read
	if over, _, err := websocket.Dial(actx, aurl, &websocket.DialOptions{HTTPClient: api.hc}); err != nil {
		t.Fatal(err)
	} else if _, _, err := over.Read(actx); websocket.CloseStatus(err) != 4031 {
		t.Fatalf("a second assistant on one session: want close 4031, got %v", err)
	}
	// input is forwarded (a bad one is dropped by the panel), and new settings reach the open window: a change of the
	// colour mode makes the helper send the whole picture again
	if err := ac.Write(actx, websocket.MessageBinary, append([]byte{assist.TypeInput}, `{"t":"m","x":300,"y":200}`...)); err != nil {
		t.Fatal(err)
	}
	if err := ac.Write(actx, websocket.MessageBinary, append([]byte{assist.TypeInput}, `{"t":"m","x":-9}`...)); err != nil {
		t.Fatal(err)
	}
	// the clipboard goes both ways and the choice of a display gives a new hello; a clipboard that is too long is dropped
	clipReq := append([]byte{assist.TypeClipSet}, `{"text":"from the browser"}`...)
	if err := ac.Write(actx, websocket.MessageBinary, clipReq); err != nil {
		t.Fatal(err)
	}
	if err := ac.Write(actx, websocket.MessageBinary, []byte{assist.TypeClipGet}); err != nil {
		t.Fatal(err)
	}
	gotClip := ""
	var gotTyped *assist.Typed
	readAssistUntil := func(until func() bool) {
		for !until() {
			_, b, err := ac.Read(actx)
			if err != nil {
				t.Fatalf("assistant frames: %v", err)
			}
			switch b[0] {
			case assist.TypeClip:
				var c assist.Clip
				_ = json.Unmarshal(b[1:], &c)
				gotClip = c.Text
			case assist.TypeTyped:
				var ty assist.Typed
				_ = json.Unmarshal(b[1:], &ty)
				gotTyped = &ty
			case assist.TypeHello:
				gotHello = bytes.Contains(b[1:], []byte(`"w":960`))
				if bytes.Contains(b[1:], []byte(`"monitor":1`)) {
					gotMonitor = true
				}
			}
		}
	}
	readAssistUntil(func() bool { return gotClip == "from the browser" })
	// a text for the sign-in or UAC screen passes the panel (a bad one does not) and is audited without its text; the helper
	// refuses it on the usual desktop and says so
	if err := ac.Write(actx, websocket.MessageBinary, append([]byte{assist.TypeType}, `{"text":"s3cret-text-42","after":"enter","label":"Home Pass"}`...)); err != nil {
		t.Fatal(err)
	}
	if err := ac.Write(actx, websocket.MessageBinary, append([]byte{assist.TypeType}, `{"text":"bad\u0007"}`...)); err != nil {
		t.Fatal(err)
	}
	readAssistUntil(func() bool { return gotTyped != nil })
	if gotTyped.OK || gotTyped.Reason != "desktop" {
		t.Fatalf("typing on the usual desktop: %+v", gotTyped)
	}
	if err := ac.Write(actx, websocket.MessageBinary, append([]byte{assist.TypeMonitor}, `{"i":1}`...)); err != nil {
		t.Fatal(err)
	}
	readAssistUntil(func() bool { return gotMonitor })
	if err := ac.Write(actx, websocket.MessageBinary, append([]byte{assist.TypeMonitor}, `{"i":1}`...)); err != nil {
		t.Fatal(err)
	}
	before := fullPictures
	if code := api.do("PUT", "/api/servers/"+id+"/assistant/settings", assist.Settings{Colors: assist.Colors16, FPS: 20, NoLimit: true, Mbit: 4}, nil); code != 200 {
		t.Fatalf("change the settings of an open assistant: %d", code)
	}
	readAssist(func() bool { return fullPictures > before })
	_ = ac.Close(websocket.StatusNormalClosure, "")
	acancel()
	waitFor(t, "assistant audit entries", func() bool {
		var n int
		_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action IN ('assistant.open','assistant.close','assistant.settings')`).Scan(&n)
		return n >= 4
	})
	var typedRows, leaks int
	_ = db.Pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE details::text LIKE '%s3cret%') FROM audit_log WHERE action = 'assistant.type' AND details->>'label' = 'Home Pass' AND (details->>'chars')::int = 14`).Scan(&typedRows, &leaks)
	if typedRows != 1 || leaks != 0 {
		t.Fatalf("audit of the typing: %d rows, %d with the text (want 1 and 0)", typedRows, leaks)
	}

	// --- files: list, edit with conflict detection, create, upload, download, delete ---
	fdir := t.TempDir()
	var fl struct {
		Entries []struct{ Name, Type string } `json:"entries"`
	}
	if code := api.do("POST", "/api/servers/"+id+"/files/mkdir", map[string]any{"path": fdir + "/sub"}, nil); code != 200 {
		t.Fatalf("mkdir: %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/files/create", map[string]any{"path": fdir + "/a.txt"}, nil); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/files/create", map[string]any{"path": fdir + "/a.txt"}, nil); code != http.StatusConflict {
		t.Fatalf("creating an existing file must conflict, got %d", code)
	}
	var fc struct {
		Content string `json:"content"`
		MTime   int64  `json:"mtime"`
	}
	api.do("GET", "/api/servers/"+id+"/files/content?path="+fdir+"/a.txt", nil, &fc)
	if code := api.do("PUT", "/api/servers/"+id+"/files/content", map[string]any{"path": fdir + "/a.txt", "content": "hello\n", "mtime": fc.MTime}, nil); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if code := api.do("PUT", "/api/servers/"+id+"/files/content", map[string]any{"path": fdir + "/a.txt", "content": "stale", "mtime": fc.MTime}, nil); code != http.StatusConflict {
		t.Fatalf("a stale mtime must be refused, got %d", code)
	}
	api.do("GET", "/api/servers/"+id+"/files?path="+fdir, nil, &fl)
	if len(fl.Entries) != 2 || fl.Entries[0].Type != "dir" || fl.Entries[1].Name != "a.txt" {
		t.Fatalf("listing: %+v", fl.Entries)
	}
	payload := bytes.Repeat([]byte("0123456789abcdef"), 40000) // 640 KB, several chunks
	upReq, _ := http.NewRequest("POST", api.base+"/api/servers/"+id+"/files/upload?path="+fdir+"/big.bin", bytes.NewReader(payload))
	upReq.Header.Set("X-CSRF-Token", api.csrf)
	if resp, err := api.hc.Do(upReq); err != nil || resp.StatusCode != 200 {
		t.Fatalf("upload: %v %v", err, resp)
	}
	dlReq, _ := http.NewRequest("GET", api.base+"/api/servers/"+id+"/files/download?path="+fdir+"/big.bin", nil)
	dl, err := api.hc.Do(dlReq)
	if err != nil || dl.StatusCode != 200 {
		t.Fatalf("download: %v", err)
	}
	dlBody, _ := io.ReadAll(dl.Body)
	_ = dl.Body.Close()
	if !bytes.Equal(dlBody, payload) {
		t.Fatalf("downloaded %d bytes, want %d identical bytes", len(dlBody), len(payload))
	}
	if code := api.do("POST", "/api/servers/"+id+"/files/delete", map[string]any{"path": fdir + "/big.bin"}, nil); code != http.StatusConflict {
		t.Fatalf("delete without confirmation must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/files/delete", map[string]any{"path": fdir + "/big.bin", "confirm": true}, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/files/delete", map[string]any{"path": "/etc", "confirm": true}, nil); code == 200 {
		t.Fatal("deleting /etc must be refused")
	}
	// --- network and firewall state are readable; apply needs a draft ---
	if code := api.do("GET", "/api/servers/"+id+"/network", nil, nil); code != 200 {
		t.Fatalf("network state: %d", code)
	}
	if code := api.do("GET", "/api/servers/"+id+"/firewall", nil, nil); code != 200 {
		t.Fatalf("firewall state: %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/network/apply", map[string]any{"confirm": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("applying without a draft must be refused, got %d", code)
	}
	if code := api.do("PUT", "/api/servers/"+id+"/firewall/draft", map[string]any{"chains": []map[string]any{{"name": "prerouting", "policy": "drop", "rules": []any{}}}}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("an unmanaged chain must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/servers/"+id+"/network/confirm", map[string]any{"id": "nope"}, nil); code != http.StatusGone {
		t.Fatalf("confirming an unknown change must answer 410, got %d", code)
	}

	// --- deleting a client ---
	// Re-read the name: it follows the hostname the agent reports, and the fake collector reports "h".
	var cur struct {
		Name string `json:"name"`
	}
	api.do("GET", "/api/servers/"+id, nil, &cur)
	name := cur.Name
	if code := api.do("DELETE", "/api/servers/"+id, map[string]any{"name": "nope", "uninstall": true}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("delete with wrong confirmation name: %d", code)
	}
	var del struct {
		Deleted       bool `json:"deleted"`
		UninstallSent bool `json:"uninstall_sent"`
	}
	if code := api.do("DELETE", "/api/servers/"+id, map[string]any{"name": name, "uninstall": true}, &del); code != 200 || !del.Deleted || !del.UninstallSent {
		t.Fatalf("delete: code=%d %+v", code, del)
	}
	select {
	case err := <-runErr:
		if !errors.Is(err, agent.ErrUninstalled) || !uninstalled.ran || !uninstalled.purge {
			t.Fatalf("agent must uninstall itself: err=%v %+v", err, uninstalled)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("agent did not react to the uninstall command")
	}
	api.do("GET", "/api/servers", nil, &servers)
	if len(servers) != 0 {
		t.Fatalf("server must be gone: %+v", servers)
	}

	// --- a deleted agent that still has its certificate is rejected as revoked ---
	err = agent.Run(ctx, agent.RunOptions{DataDir: adata, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if !errors.Is(err, agent.ErrRevoked) {
		t.Fatalf("expected ErrRevoked, got %v", err)
	}

	// --- audit log ---
	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action IN ('auth.login','enroll.approve','server.delete')`).Scan(&n); err != nil || n < 4 {
		t.Fatalf("audit entries: %d err=%v", n, err)
	}

	// --- brute-forcing the enrollment key rejects the request ---
	bf := t.TempDir()
	shown2 := make(chan shown, 1)
	go func() {
		_ = agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: serverFP, DataDir: bf, PollEvery: 100 * time.Millisecond,
			Show: func(k, c, _ string) { shown2 <- shown{k, c} }})
	}()
	<-shown2
	waitFor(t, "second pending", func() bool { api.do("GET", "/api/enrollments", nil, &pending); return len(pending) == 1 })
	var last int
	for i := 0; i < 5; i++ {
		last = api.do("POST", "/api/enrollments/"+pending[0].ID+"/approve", map[string]string{"key": fmt.Sprintf("AAAA-%04d", i)}, nil)
	}
	if last != http.StatusGone {
		t.Fatalf("5th wrong key must reject the request (410), got %d", last)
	}

	cancel()
	select {
	case err := <-srvDone:
		if err != nil {
			t.Fatalf("server stopped with error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestSecurityHeadersAndTOTPGate(t *testing.T) {
	dsn := os.Getenv("LINAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("LINAGER_TEST_DSN is not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)
	dir := t.TempDir()
	cfg := &config.Server{DataDir: dir}
	cfg.Database.DSN = dsn
	cfg.App.Listen, cfg.Agents.Listen = freeAddr(t), freeAddr(t)
	cfg.Agents.Hosts = []string{"127.0.0.1"}
	cfg.Admin.CookieSecure = "never"
	cfg.Admin.RequireTOTP = true
	if err := server.InitKeys(dir, cfg.Agents.Hosts); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := server.CreateAdmin(ctx, db, "admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	waitFor(t, "server", func() bool {
		c, e := net.Dial("tcp", cfg.App.Listen)
		if e == nil {
			_ = c.Close()
		}
		return e == nil
	})

	jar, _ := cookiejar.New(nil)
	api := &apiClient{t: t, base: "https://" + cfg.App.Listen, hc: testClient(jar)}
	var lr struct{ CSRF string }
	api.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password"}, &lr)
	api.csrf = lr.CSRF

	// the TOTP gate: everything except the 2FA setup endpoints is refused until 2FA is on
	if code := api.do("GET", "/api/servers", nil, nil); code != http.StatusForbidden {
		t.Fatalf("with require_totp the API must be closed until 2FA is enabled, got %d", code)
	}
	var me struct {
		Required bool `json:"totp_required"`
	}
	if code := api.do("GET", "/api/me", nil, &me); code != 200 || !me.Required {
		t.Fatalf("me must announce the requirement: %d %+v", code, me)
	}
	var setup struct{ Secret string }
	if code := api.do("POST", "/api/me/totp/setup", nil, &setup); code != 200 || setup.Secret == "" {
		t.Fatalf("setup must stay reachable: %d", code)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if c := api.do("POST", "/api/me/totp/enable", map[string]string{"code": code}, nil); c != 200 {
		t.Fatalf("enable: %d", c)
	}
	if c := api.do("GET", "/api/servers", nil, nil); c != 200 {
		t.Fatalf("after enabling 2FA the API must open, got %d", c)
	}

	// two-step sign-in: the password alone only asks for the code (no session, not a failed attempt),
	// a wrong code is refused, a fresh right one opens the session
	jar2, _ := cookiejar.New(nil)
	api2 := &apiClient{t: t, base: api.base, hc: testClient(jar2)}
	var first struct {
		Error string `json:"error"`
	}
	if c := api2.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password"}, &first); c != http.StatusUnauthorized || first.Error != server.ErrTOTPRequired {
		t.Fatalf("password without a code must answer %q, got %d %+v", server.ErrTOTPRequired, c, first)
	}
	if c := api2.do("GET", "/api/servers", nil, nil); c != http.StatusUnauthorized {
		t.Fatalf("the first stage must not create a session, got %d", c)
	}
	var bad struct {
		Error string `json:"error"`
	}
	if c := api2.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password", "totp": "000000"}, &bad); c != http.StatusUnauthorized || bad.Error == server.ErrTOTPRequired {
		t.Fatalf("a wrong code must be refused as invalid credentials, got %d %+v", c, bad)
	}
	next, _ := totp.GenerateCode(setup.Secret, time.Now().Add(30*time.Second)) // a step newer than the one used to enable
	var ok struct{ CSRF string }
	if c := api2.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password", "totp": next}, &ok); c != 200 || ok.CSRF == "" {
		t.Fatalf("password and a fresh code must sign in: %d", c)
	}
	if c := api2.do("GET", "/api/servers", nil, nil); c != 200 {
		t.Fatalf("second stage session must work, got %d", c)
	}

	// security headers on every response
	resp, err := api.hc.Get(api.base + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	for _, h := range []string{"X-Content-Type-Options", "Cache-Control"} {
		if resp.Header.Get(h) == "" {
			t.Errorf("missing security header %s", h)
		}
	}
	if resp.Header.Get("Strict-Transport-Security") == "" {
		t.Error("HSTS must be sent: the application port is TLS")
	}
}
