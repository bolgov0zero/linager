package server_test

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
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

type joinCmd struct{ Kind, Host, Command, Platform string }
type joinMade struct {
	ID       int64
	Label    string
	Token    string
	Commands []joinCmd
}

// TestInstallLinks covers the install by a link: the commands, the script, enrolling with a token that needs a
// click and with one that approves itself, the limits of a link and its revocation.
func TestInstallLinks(t *testing.T) {
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
	cfg.Agents.Hosts = []string{"127.0.0.1", "203.0.113.5", "10.9.9.9"}
	cfg.Admin.CookieSecure = "never"
	if err := server.InitKeys(dir, cfg.Agents.Hosts); err != nil {
		t.Fatal(err)
	}
	keys, err := server.LoadKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	fp := pki.Fingerprint(keys.TLSDER)
	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := server.CreateAdmin(ctx, db, "admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	waitFor(t, "server listening", func() bool {
		c, err := net.Dial("tcp", cfg.Agents.Listen)
		if err == nil {
			_ = c.Close()
		}
		return err == nil
	})
	jar, _ := cookiejar.New(nil)
	api := &apiClient{t: t, base: "https://" + cfg.App.Listen, hc: testClient(jar)}
	var lr struct{ CSRF string }
	if code := api.do("POST", "/api/login", map[string]string{"login": "admin", "password": "a-long-test-password"}, &lr); code != 200 {
		t.Fatalf("login: %d", code)
	}
	api.csrf = lr.CSRF
	var grp struct{ ID int64 }
	if code := api.do("POST", "/api/groups", map[string]string{"name": "Linked"}, &grp); code != 200 {
		t.Fatalf("group: %d", code)
	}

	// invalid requests are refused
	for _, body := range []map[string]any{
		{"ttl_seconds": 10, "max_uses": 1},
		{"ttl_seconds": 3600, "max_uses": -1},
		{"ttl_seconds": 3600, "max_uses": 1, "networks": []string{"not-a-network"}},
		{"ttl_seconds": 3600, "max_uses": 1, "group_id": 99999},
	} {
		if code := api.do("POST", "/api/join-tokens", body, nil); code != http.StatusUnprocessableEntity {
			t.Fatalf("%v must be refused, got %d", body, code)
		}
	}

	// a link that needs a click
	var l1 joinMade
	if code := api.do("POST", "/api/join-tokens", map[string]any{"ttl_seconds": 3600, "max_uses": 1, "group_id": grp.ID}, &l1); code != 200 || !strings.HasPrefix(l1.Token, "lk_") {
		t.Fatalf("create link: %d %+v", code, l1)
	}
	kinds := map[string]joinCmd{}
	winCmds := 0
	for _, c := range l1.Commands {
		if c.Platform == "windows" {
			winCmds++
			if !strings.Contains(c.Command, "/v1/join/"+l1.Token+"/windows") || !strings.Contains(c.Command, "LinagerPin") || strings.Contains(c.Command, "\n") ||
				!strings.Contains(c.Command, "[LinagerPin]::Fp='"+pki.Fingerprint(keys.TLSAltDER)+"'") {
				t.Fatalf("the Windows command: %s", c.Command)
			}
			continue
		}
		kinds[c.Kind] = c
	}
	if winCmds != 2 {
		t.Fatalf("a Windows command for each address: %d", winCmds)
	}
	pin, _ := server.SPKIPin(keys.TLSDER)
	if kinds["external"].Host != "203.0.113.5" || kinds["local"].Host != "10.9.9.9" ||
		!strings.Contains(kinds["external"].Command, pin) || !strings.Contains(kinds["local"].Command, "/v1/join/"+l1.Token) || !strings.HasPrefix(kinds["local"].Command, "wget -nv -O- --no-check-certificate --pinnedpubkey ") || strings.Contains(kinds["local"].Command, "sudo") || !strings.HasSuffix(kinds["local"].Command, " | sh") {
		t.Fatalf("commands: %+v", l1.Commands)
	}
	var list []struct {
		ID    int64
		Label string
		Uses  int
	}
	if code := api.do("GET", "/api/join-tokens", nil, &list); code != 200 || len(list) != 1 || list[0].Label != l1.Label || strings.Contains(l1.Label, l1.Token) {
		t.Fatalf("list: %d %+v", code, list)
	}

	// the script comes from the agent port, needs the token and shows what it does
	pinned := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec // a test against the server's own certificate
	// a client without TLS 1.3 (an older Windows PowerShell) may fetch the install link and nothing else
	old := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12}}} //nolint:gosec // a test against the server's own certificate
	if resp, err := old.Get("https://" + cfg.Agents.Listen + "/v1/join/" + l1.Token); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("a TLS 1.2 client must get the install script: %v %v", err, resp)
	} else {
		_ = resp.Body.Close()
	}
	if resp, err := old.Get("https://" + cfg.Agents.Listen + "/v1/release/latest"); err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a TLS 1.2 client must not reach the agent routes: %v %v", err, resp)
	} else {
		_ = resp.Body.Close()
	}
	get := func(path string) (int, string) {
		resp, err := pinned.Get("https://" + cfg.Agents.Listen + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	code, script := get("/v1/join/" + l1.Token)
	if code != 200 || !strings.Contains(script, "--join-token") || !strings.Contains(script, pin) || !strings.Contains(script, fp) || !strings.Contains(script, "SERVER='"+cfg.Agents.Listen+"'") {
		t.Fatalf("script: %d\n%s", code, script)
	}
	if code, _ := get("/v1/join/lk_" + strings.Repeat("A", 22)); code != http.StatusNotFound {
		t.Fatalf("an unknown token must give 404, got %d", code)
	}
	if code, _ := get("/v1/join/garbage"); code != http.StatusNotFound {
		t.Fatalf("a malformed token must give 404, got %d", code)
	}

	// enrolling without a token still works the old way (covered by TestEndToEnd); with the token it needs a click
	adata := t.TempDir()
	done := make(chan error, 1)
	go func() {
		done <- agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: fp, DataDir: adata, AgentVersion: "0.0.1-test", JoinToken: l1.Token,
			PollEvery: 100 * time.Millisecond, Show: func(string, string, string) {}})
	}()
	var pending []struct {
		ID       string `json:"id"`
		Via      string `json:"via"`
		NeedsKey bool   `json:"needs_key"`
	}
	waitFor(t, "a pending request from the link", func() bool { api.do("GET", "/api/enrollments", nil, &pending); return len(pending) == 1 })
	if pending[0].Via != l1.Label || pending[0].NeedsKey {
		t.Fatalf("the request must name the link and need no key: %+v", pending[0])
	}
	if code := api.do("POST", "/api/enrollments/"+pending[0].ID+"/approve", map[string]string{}, nil); code != 200 {
		t.Fatalf("approve without a key: %d", code)
	}
	if err := <-done; err != nil {
		t.Fatalf("enroll: %v", err)
	}
	var srvs []struct {
		ID      string
		GroupID *int64 `json:"group_id"`
	}
	if code := api.do("GET", "/api/servers", nil, &srvs); code != 200 || len(srvs) != 1 || srvs[0].GroupID == nil || *srvs[0].GroupID != grp.ID {
		t.Fatalf("the server must be in the group of the link: %d %+v", code, srvs)
	}
	// the single use is spent
	err = agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: fp, DataDir: t.TempDir(), JoinToken: l1.Token, Show: func(string, string, string) {}})
	if err == nil || !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("a spent link must be refused, got %v", err)
	}
	if code, _ := get("/v1/join/" + l1.Token); code != http.StatusNotFound {
		t.Fatalf("a spent link must give 404, got %d", code)
	}

	// a link that approves itself and is limited to a network that this test is not in
	var l2 joinMade
	if code := api.do("POST", "/api/join-tokens", map[string]any{"ttl_seconds": 3600, "max_uses": 5, "auto_approve": true, "networks": []string{"203.0.113.0/24"}}, &l2); code != 200 {
		t.Fatalf("create link 2: %d", code)
	}
	err = agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: fp, DataDir: t.TempDir(), JoinToken: l2.Token, Show: func(string, string, string) {}})
	if err == nil {
		t.Fatal("a link limited to another network must be refused")
	}
	if code, _ := get("/v1/join/" + l2.Token); code != http.StatusNotFound {
		t.Fatalf("the script of a link limited to another network must give 404, got %d", code)
	}
	// revoke it, make an open one that approves itself
	if code := api.do("DELETE", "/api/join-tokens/"+itoa(l2.ID), nil, nil); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	var l3 joinMade
	if code := api.do("POST", "/api/join-tokens", map[string]any{"ttl_seconds": 3600, "max_uses": 0, "auto_approve": true}, &l3); code != 200 {
		t.Fatalf("create link 3: %d", code)
	}
	if err := agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: fp, DataDir: t.TempDir(), JoinToken: l3.Token, PollEvery: 100 * time.Millisecond,
		Show: func(string, string, string) {}}); err != nil {
		t.Fatalf("a link that approves itself must pair without a click: %v", err)
	}
	if code := api.do("GET", "/api/servers", nil, &srvs); code != 200 || len(srvs) != 2 {
		t.Fatalf("two servers expected: %d %+v", code, srvs)
	}
	// revoked links are gone from the list and refuse agents
	if code := api.do("DELETE", "/api/join-tokens/"+itoa(l3.ID), nil, nil); code != 200 {
		t.Fatalf("revoke 3: %d", code)
	}
	if err := agent.Enroll(ctx, agent.EnrollOptions{Server: cfg.Agents.Listen, Fingerprint: fp, DataDir: t.TempDir(), JoinToken: l3.Token, Show: func(string, string, string) {}}); err == nil {
		t.Fatal("a revoked link must be refused")
	}
	if code := api.do("GET", "/api/join-tokens", nil, &list); code != 200 || len(list) != 0 {
		t.Fatalf("no active link expected: %d %+v", code, list)
	}

	// the SSH install refuses what it must before it opens a connection
	for _, body := range []map[string]any{
		{"host": "127.0.0.1", "login": "u", "auth": "password", "password": "x"},       // loopback
		{"host": "169.254.169.254", "login": "u", "auth": "password", "password": "x"}, // cloud metadata
		{"host": "10.0.0.5", "login": "bad login", "auth": "password", "password": "x"},
		{"host": "10.0.0.5", "login": "u", "auth": "key"},
		{"host": "10.0.0.5", "login": "u", "auth": "other", "password": "x"},
		{"host": "10.0.0.5", "port": 70000, "login": "u", "auth": "password", "password": "x"},
	} {
		if code := api.do("POST", "/api/ssh-install", body, nil); code != http.StatusUnprocessableEntity {
			t.Fatalf("%v must be refused, got %d", body, code)
		}
	}
	if code := api.do("GET", "/api/ssh-install/nothing", nil, nil); code != http.StatusNotFound {
		t.Fatalf("an unknown install: %d", code)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
