package server_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/release"
	"github.com/bolgov0zero/linager/internal/version"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

func TestUpdatesAndAuditAPI(t *testing.T) {
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

	// a fake release server signed with a throw-away key; the "embedded" key is swapped for the test
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	key := &release.Key{Pub: pub}
	copy(key.ID[:], "e2e-key1")
	oldKey, oldVer := release.PublicKey, version.Version
	raw := append([]byte("Ed"), key.ID[:]...)
	raw = append(raw, pub...)
	release.PublicKey = base64.StdEncoding.EncodeToString(raw)
	version.Version = "v0.1.0"
	t.Cleanup(func() { release.PublicKey, version.Version = oldKey, oldVer })

	files := map[string][]byte{}
	var rels []map[string]any
	publish := func(tag string, binary string) {
		sum := sha256.Sum256([]byte(binary))
		m, _ := json.Marshal(release.Manifest{Version: tag, Channel: "stable", MinAgent: "v0.1.0", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(binary)), Migrations: 99, Notes: []string{"n"}})
		sig := ed25519.Sign(priv, m)
		sf := append(append([]byte("Ed"), key.ID[:]...), sig...)
		glob := ed25519.Sign(priv, append(append([]byte(nil), sig...), "t"...))
		files[tag+"/bin"], files[tag+"/m"] = []byte(binary), m
		files[tag+"/s"] = []byte("untrusted comment: x\n" + base64.StdEncoding.EncodeToString(sf) + "\ntrusted comment: t\n" + base64.StdEncoding.EncodeToString(glob) + "\n")
	}
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/releases.json", func(w http.ResponseWriter, _ *http.Request) {
		var out []map[string]any
		for _, tag := range []string{"v0.1.0", "v0.2.0"} {
			u := func(n string) string { return base + "/dl/" + tag + "/" + n }
			out = append(out, map[string]any{"tag_name": tag, "prerelease": false, "body": "- change one\n- change two",
				"assets": []map[string]string{{"name": release.BinaryAsset(release.ArchAMD64), "browser_download_url": u("bin")}, {"name": "manifest.json", "browser_download_url": u("m")}, {"name": "manifest.json.minisig", "browser_download_url": u("s")}}})
		}
		_ = rels
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[strings.TrimPrefix(r.URL.Path, "/dl/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	})
	fake := httptest.NewServer(mux)
	defer fake.Close()
	base = fake.URL
	publish("v0.1.0", "binary-one")
	publish("v0.2.0", "binary-two")

	dir := t.TempDir()
	cfg := &config.Server{DataDir: dir}
	cfg.Database.DSN = dsn
	cfg.App.Listen, cfg.Agents.Listen = freeAddr(t), freeAddr(t)
	cfg.Agents.Hosts = []string{"127.0.0.1"}
	cfg.Admin.CookieSecure = "never"
	cfg.Update.Source = fake.URL
	cfg.Update.Channel = "stable"
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

	type overview struct {
		Channel string `json:"channel"`
		Server  struct {
			Current   string  `json:"current"`
			Available *string `json:"available"`
			Notes     []string
			Mig       struct{ Pending int } `json:"db_migrations"`
			CanUpdate bool                  `json:"can_update"`
		} `json:"server"`
		Cache []struct {
			Version    string
			Verified   bool
			Downloaded bool
		} `json:"cache"`
	}
	var ov overview
	waitFor(t, "release check", func() bool {
		api.do("POST", "/api/updates/check", nil, &ov)
		return ov.Server.Available != nil
	})
	if *ov.Server.Available != "v0.2.0" || ov.Server.Current != "v0.1.0" || len(ov.Server.Notes) != 2 || ov.Channel != "stable" {
		t.Fatalf("overview: %+v", ov)
	}
	if ov.Server.Mig.Pending == 0 {
		t.Fatal("the manifest announces more migrations than applied: pending must be shown")
	}
	var sum struct{ Count int }
	api.do("GET", "/api/updates/summary", nil, &sum)
	if sum.Count != 1 {
		t.Fatalf("summary counts the available server update: %d", sum.Count)
	}
	if code := api.do("POST", "/api/updates/cache", map[string]string{"version": "v0.2.0"}, &ov); code != 200 {
		t.Fatalf("prefetch: %d", code)
	}
	found := false
	for _, c := range ov.Cache {
		found = found || (c.Version == "v0.2.0" && c.Verified && c.Downloaded)
	}
	if !found {
		t.Fatalf("a prefetched release must be listed as verified: %+v", ov.Cache)
	}
	if code := api.do("POST", "/api/updates/cache", map[string]string{"version": "../../etc/passwd"}, nil); code != http.StatusBadRequest {
		t.Fatalf("path-like versions must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/updates/server", map[string]any{"version": "v0.2.0"}, nil); code != http.StatusConflict {
		t.Fatalf("updating without confirmation must be refused, got %d", code)
	}
	if code := api.do("POST", "/api/updates/server", map[string]any{"version": "v0.2.0", "confirm": true}, nil); code != http.StatusConflict {
		t.Fatalf("without the updater unit the request must be refused with a clear reason, got %d", code)
	}
	if code := api.do("PUT", "/api/updates/channel", map[string]string{"channel": "nightly"}, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown channel: %d", code)
	}
	if code := api.do("PUT", "/api/updates/channel", map[string]string{"channel": "beta"}, &ov); code != 200 || ov.Channel != "beta" {
		t.Fatalf("channel switch: %d %s", code, ov.Channel)
	}
	if code := api.do("POST", "/api/updates/agents", map[string]any{"all": true}, nil); code != http.StatusConflict {
		t.Fatalf("a rollout with no agents must be refused, got %d", code)
	}

	// audit log: filters and paging
	var au struct {
		Items []struct {
			Action, Result string
			Actor          string
		} `json:"items"`
		NextBefore int64 `json:"next_before"`
	}
	if code := api.do("GET", "/api/audit?action=update.&limit=50", nil, &au); code != 200 || len(au.Items) < 4 {
		t.Fatalf("audit: %d %+v", code, au.Items)
	}
	for _, it := range au.Items {
		if !strings.HasPrefix(it.Action, "update.") || it.Actor != "admin" {
			t.Fatalf("filter leaked: %+v", it)
		}
	}
	if code := api.do("GET", "/api/audit?action=auth.,update.channel&limit=50", nil, &au); code != 200 || len(au.Items) == 0 {
		t.Fatalf("comma-separated prefixes: %d %+v", code, au.Items)
	}
	for _, it := range au.Items {
		if !strings.HasPrefix(it.Action, "auth.") && !strings.HasPrefix(it.Action, "update.channel") {
			t.Fatalf("prefix list leaked: %+v", it)
		}
	}
	api.do("GET", "/api/audit?result=failed", nil, &au)
	if len(au.Items) == 0 {
		t.Fatal("failed update attempts must be in the audit log")
	}
	api.do("GET", "/api/audit?limit=2", nil, &au)
	if len(au.Items) != 2 {
		t.Fatalf("limit: %d", len(au.Items))
	}
	_ = time.Now
}
