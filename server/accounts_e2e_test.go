package server_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

// startAccountsServer runs a server on a throw-away database and returns the base URL of its application port (host
// "localhost") and the database. The admin is "admin" / "a-long-test-password".
func startAccountsServer(t *testing.T) (base string, db *store.Store) {
	t.Helper()
	dsn := os.Getenv("LINAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("LINAGER_TEST_DSN is not set")
	}
	if !strings.Contains(dsn, "test") {
		t.Fatal("refusing to wipe a database whose DSN does not contain 'test'")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
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
	cfg.Admin.LoginRate = 1000 // the tests sign in many times a minute
	if err := server.InitKeys(dir, cfg.Agents.Hosts); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := server.CreateAdmin(ctx, db, "admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx, cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
		close(done)
	}()
	t.Cleanup(func() { // the next test wipes the database: the server must be gone by then
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	})
	waitFor(t, "server", func() bool {
		c, e := net.Dial("tcp", cfg.App.Listen)
		if e == nil {
			_ = c.Close()
		}
		return e == nil
	})
	_, port, _ := net.SplitHostPort(cfg.App.Listen)
	return "https://localhost:" + port, db
}

// testClient talks to the application port of a test server: TLS with the certificate of the server itself.
func testClient(jar http.CookieJar) *http.Client {
	return &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}}} //nolint:gosec // a test server with its own certificate
}

func newClient(t *testing.T, base string) *apiClient {
	jar, _ := cookiejar.New(nil)
	return &apiClient{t: t, base: base, hc: testClient(jar)}
}

func (c *apiClient) login(login, password string, extra map[string]string) (int, map[string]any) {
	body := map[string]string{"login": login, "password": password}
	for k, v := range extra {
		body[k] = v
	}
	var out map[string]any
	code := c.do("POST", "/api/login", body, &out)
	if s, ok := out["csrf"].(string); ok {
		c.csrf = s
	}
	return code, out
}

func mustLogin(t *testing.T, base, login, password string) *apiClient {
	t.Helper()
	c := newClient(t, base)
	if code, _ := c.login(login, password, nil); code != 200 {
		t.Fatalf("login %s: %d", login, code)
	}
	return c
}

func TestUsersPasswordRecoveryAndSnippets(t *testing.T) {
	base, _ := startAccountsServer(t)
	admin := mustLogin(t, base, "admin", "a-long-test-password")

	// --- users ---
	if c := admin.do("POST", "/api/users", map[string]string{"login": "Second", "password": "second-long-password"}, nil); c != 200 {
		t.Fatalf("create user: %d", c)
	}
	if c := admin.do("POST", "/api/users", map[string]string{"login": "second", "password": "second-long-password"}, nil); c != http.StatusConflict {
		t.Fatalf("duplicate login must be refused: %d", c)
	}
	if c := admin.do("POST", "/api/users", map[string]string{"login": "third", "password": "short"}, nil); c != http.StatusUnprocessableEntity {
		t.Fatalf("short password: %d", c)
	}
	if c := admin.do("POST", "/api/users", map[string]string{"login": "x y", "password": "third-long-password"}, nil); c != http.StatusUnprocessableEntity {
		t.Fatalf("bad login: %d", c)
	}
	var list struct {
		Users []struct {
			ID      int64  `json:"id"`
			Login   string `json:"login"`
			Current bool   `json:"current"`
		}
	}
	admin.do("GET", "/api/users", nil, &list)
	if len(list.Users) != 2 {
		t.Fatalf("users: %+v", list)
	}
	var selfID, secondID int64
	for _, u := range list.Users {
		if u.Current {
			selfID = u.ID
		}
		if u.Login == "second" {
			secondID = u.ID
		}
	}
	if c := admin.do("DELETE", fmt.Sprintf("/api/users/%d", selfID), map[string]bool{"confirm": true}, nil); c != http.StatusConflict {
		t.Fatalf("deleting yourself must be refused: %d", c)
	}
	if c := admin.do("DELETE", fmt.Sprintf("/api/users/%d", secondID), map[string]bool{}, nil); c != http.StatusConflict {
		t.Fatalf("deleting needs a confirmation: %d", c)
	}

	// --- own password: wrong current refused, other sessions end ---
	second := mustLogin(t, base, "second", "second-long-password")
	other := mustLogin(t, base, "second", "second-long-password")
	if c := second.do("POST", "/api/me/password", map[string]string{"current": "wrong-wrong-wrong", "password": "brand-new-password"}, nil); c != http.StatusForbidden {
		t.Fatalf("wrong current password: %d", c)
	}
	if c := second.do("POST", "/api/me/password", map[string]string{"current": "second-long-password", "password": "brand-new-password"}, nil); c != 200 {
		t.Fatalf("change password: %d", c)
	}
	if c := second.do("GET", "/api/me", nil, nil); c != 200 {
		t.Fatalf("the session that changed the password stays: %d", c)
	}
	if c := other.do("GET", "/api/me", nil, nil); c != http.StatusUnauthorized {
		t.Fatalf("the other session must end: %d", c)
	}
	if code, _ := newClient(t, base).login("second", "second-long-password", nil); code != http.StatusUnauthorized {
		t.Fatalf("the old password must stop working: %d", code)
	}

	// --- reset by an administrator ends the sessions of that user ---
	if c := admin.do("POST", fmt.Sprintf("/api/users/%d/password", secondID), map[string]any{"password": "reset-by-admin-pass"}, nil); c != http.StatusConflict { //nolint:gosec // test password
		t.Fatalf("reset needs a confirmation: %d", c)
	}
	if c := admin.do("POST", fmt.Sprintf("/api/users/%d/password", secondID), map[string]any{"password": "reset-by-admin-pass", "confirm": true}, nil); c != 200 { //nolint:gosec // test password
		t.Fatalf("reset: %d", c)
	}
	if c := second.do("GET", "/api/me", nil, nil); c != http.StatusUnauthorized {
		t.Fatalf("a reset must end the sessions of the user: %d", c)
	}
	second = mustLogin(t, base, "second", "reset-by-admin-pass")

	// --- 2FA with recovery codes ---
	var setup struct{ Secret string }
	second.do("POST", "/api/me/totp/setup", nil, &setup)
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	var en struct {
		Codes []string `json:"recovery_codes"`
	}
	if c := second.do("POST", "/api/me/totp/enable", map[string]string{"code": code}, &en); c != 200 || len(en.Codes) != 10 {
		t.Fatalf("enable 2FA must return 10 recovery codes: %d %v", c, en.Codes)
	}
	var me struct {
		Left int `json:"recovery_left"`
	}
	second.do("GET", "/api/me", nil, &me)
	if me.Left != 10 {
		t.Fatalf("recovery_left: %d", me.Left)
	}
	var first struct {
		Error string `json:"error"`
	}
	c1 := newClient(t, base)
	if code, out := c1.login("second", "reset-by-admin-pass", nil); code != http.StatusUnauthorized || out["error"] != server.ErrTOTPRequired {
		t.Fatalf("password alone must ask for the second factor: %d %v", code, out)
	}
	_ = first
	// a recovery code works once, typed in lower case without the dash
	typed := strings.ToLower(strings.ReplaceAll(en.Codes[0], "-", ""))
	if code, _ := c1.login("second", "reset-by-admin-pass", map[string]string{"totp": typed}); code != 200 {
		t.Fatalf("recovery code: %d", code)
	}
	if code, _ := newClient(t, base).login("second", "reset-by-admin-pass", map[string]string{"totp": en.Codes[0]}); code != http.StatusUnauthorized {
		t.Fatalf("a used recovery code must be refused: %d", code)
	}
	if code, _ := newClient(t, base).login("second", "reset-by-admin-pass", map[string]string{"totp": "AAAAA-AAAAA"}); code != http.StatusUnauthorized {
		t.Fatalf("an unknown recovery code must be refused: %d", code)
	}
	// regenerate: needs the password, replaces all codes
	if c := second.do("POST", "/api/me/recovery-codes", map[string]string{"password": "nope-nope-nope"}, nil); c != http.StatusForbidden { //nolint:gosec // test password
		t.Fatalf("regenerate needs the password: %d", c)
	}
	var re struct {
		Codes []string `json:"recovery_codes"`
	}
	if c := second.do("POST", "/api/me/recovery-codes", map[string]string{"password": "reset-by-admin-pass"}, &re); c != 200 || len(re.Codes) != 10 { //nolint:gosec // test password
		t.Fatalf("regenerate: %d", c)
	}
	if code, _ := newClient(t, base).login("second", "reset-by-admin-pass", map[string]string{"totp": en.Codes[1]}); code != http.StatusUnauthorized {
		t.Fatalf("old codes must stop working after a regeneration: %d", code)
	}
	if code, _ := newClient(t, base).login("second", "reset-by-admin-pass", map[string]string{"totp": re.Codes[0]}); code != 200 {
		t.Fatalf("a new code must work: %d", code)
	}
	// an administrator resets the 2FA of a user who lost the phone and the codes
	if c := admin.do("POST", fmt.Sprintf("/api/users/%d/2fa/reset", secondID), map[string]bool{"confirm": true}, nil); c != 200 {
		t.Fatalf("2FA reset: %d", c)
	}
	if code, _ := newClient(t, base).login("second", "reset-by-admin-pass", nil); code != 200 {
		t.Fatalf("after a 2FA reset the password alone signs in: %d", code)
	}
	if c := admin.do("DELETE", fmt.Sprintf("/api/users/%d", secondID), map[string]bool{"confirm": true}, nil); c != 200 {
		t.Fatalf("delete user: %d", c)
	}
	if code, _ := newClient(t, base).login("second", "reset-by-admin-pass", nil); code != http.StatusUnauthorized {
		t.Fatalf("a deleted user cannot sign in: %d", code)
	}

	// --- snippets ---
	var sn struct {
		ID int64 `json:"id"`
	}
	if c := admin.do("POST", "/api/snippets", map[string]string{"name": "", "command": "ls"}, nil); c != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: %d", c)
	}
	if c := admin.do("POST", "/api/snippets", map[string]string{"name": "Disk", "command": "df -h\n"}, &sn); c != 200 || sn.ID == 0 {
		t.Fatalf("create snippet: %d", c)
	}
	var sn2 struct {
		ID int64 `json:"id"`
	}
	admin.do("POST", "/api/snippets", map[string]string{"name": "Mem", "command": "free -m"}, &sn2)
	if c := admin.do("PUT", fmt.Sprintf("/api/snippets/%d", sn.ID), map[string]string{"name": "Disk usage", "command": "df -h /"}, nil); c != 200 {
		t.Fatalf("update snippet: %d", c)
	}
	if c := admin.do("PUT", "/api/snippets/order", map[string]any{"ids": []int64{sn2.ID, sn.ID}}, nil); c != 200 {
		t.Fatalf("order: %d", c)
	}
	var sl struct {
		Snippets []struct {
			ID      int64
			Name    string
			Command string
		}
	}
	admin.do("GET", "/api/snippets", nil, &sl)
	if len(sl.Snippets) != 2 || sl.Snippets[0].ID != sn2.ID || sl.Snippets[1].Name != "Disk usage" || sl.Snippets[1].Command != "df -h /" {
		t.Fatalf("snippets: %+v", sl)
	}
	if c := admin.do("POST", fmt.Sprintf("/api/snippets/%d/ran", sn.ID), map[string]string{"server_id": "x"}, nil); c != 200 {
		t.Fatalf("ran: %d", c)
	}
	if c := admin.do("DELETE", fmt.Sprintf("/api/snippets/%d", sn.ID), nil, nil); c != 200 {
		t.Fatalf("delete snippet: %d", c)
	}
	if c := admin.do("DELETE", fmt.Sprintf("/api/snippets/%d", sn.ID), nil, nil); c != http.StatusNotFound {
		t.Fatalf("deleting a deleted snippet: %d", c)
	}
}

func TestResetPasswordCLI(t *testing.T) {
	base, db := startAccountsServer(t)
	old := mustLogin(t, base, "admin", "a-long-test-password")
	ctx := context.Background()
	if err := db.RecordLoginFailure(ctx, 1, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := server.ResetPassword(ctx, db, " Admin ", "brand-new-long-password"); err != nil {
		t.Fatal(err)
	}
	if c := old.do("GET", "/api/me", nil, nil); c != http.StatusUnauthorized {
		t.Fatalf("the old session must end, got %d", c)
	}
	if code, _ := newClient(t, base).login("admin", "a-long-test-password", nil); code == 200 {
		t.Fatal("the old password still works")
	}
	mustLogin(t, base, "admin", "brand-new-long-password")
	if err := server.ResetPassword(ctx, db, "nobody", "brand-new-long-password"); err == nil {
		t.Fatal("an unknown user must be refused")
	}
}
