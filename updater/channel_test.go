package updater

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The console and `linager update` follow the channel chosen in the application, not only server.yaml.
func TestStoredChannelFollowsTheApplication(t *testing.T) {
	ctx := context.Background()
	if got := StoredChannel(ctx, nil, "stable"); got != "stable" {
		t.Fatalf("no database gives the fallback, got %q", got)
	}
	dsn := os.Getenv("LINAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("LINAGER_TEST_DSN is not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1 // one connection keeps the temporary table visible
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE TEMP TABLE settings (key text PRIMARY KEY, value jsonb NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if got := StoredChannel(ctx, pool, "stable"); got != "stable" {
		t.Fatalf("an unset setting gives the fallback, got %q", got)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settings VALUES ('updates.channel', '"beta"')`); err != nil {
		t.Fatal(err)
	}
	if got := StoredChannel(ctx, pool, "stable"); got != "beta" {
		t.Fatalf("the channel of the application must win, got %q", got)
	}
	if _, err := pool.Exec(ctx, `UPDATE settings SET value='"nightly"'`); err != nil {
		t.Fatal(err)
	}
	if got := StoredChannel(ctx, pool, "stable"); got != "stable" {
		t.Fatalf("an unknown channel gives the fallback, got %q", got)
	}
}
