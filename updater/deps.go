package updater

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bolgov0zero/linager/backup"
	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/release"
	"github.com/bolgov0zero/linager/internal/version"
	"github.com/bolgov0zero/linager/server/store"
	"github.com/bolgov0zero/linager/update"
)

// Paths used on a real installation.
const (
	BinPath = "/usr/local/bin/linager"
	// StateDir belongs to root only (0700): the staged binary and the previous binaries live here, out of
	// reach of the unprivileged server user.
	StateDir = "/var/lib/linager-updater"
)

// StatusFile, RequestFile and cache directories derive from the data directory.
func StatusFile(dataDir string) string  { return filepath.Join(dataDir, "update-status.json") }
func RequestFile(dataDir string) string { return filepath.Join(dataDir, "update-request.json") }
func CacheDir(dataDir string) string    { return filepath.Join(dataDir, "releases") }

// DefaultDeps wires the production collaborators from the server configuration.
func DefaultDeps(cfgPath string) (Deps, *config.Server, error) {
	cfg, err := config.LoadServer(cfgPath)
	if err != nil {
		return Deps{}, nil, err
	}
	key, err := release.Embedded()
	if err != nil {
		return Deps{}, nil, err
	}
	d := Deps{
		Cfg: cfg, ConfigPath: cfgPath, BinPath: BinPath,
		CacheDir: CacheDir(cfg.DataDir), PrevDir: filepath.Join(StateDir, "prev"), StageDir: StateDir,
		StatusPath: StatusFile(cfg.DataDir), RequestPath: RequestFile(cfg.DataDir),
		Source: update.NewSource(cfg.Update.Source), Key: key, CurrentVersion: version.Version,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // fixed commands built by the updater
		},
		Backup: func(ctx context.Context) (string, error) {
			return backup.Run(ctx, backup.Options{DSN: cfg.Database.DSN, OutDir: backup.DefaultDir, Files: []string{cfgPath}, DataDir: cfg.DataDir, Keep: 7, Prefix: "linager-pre-update"})
		},
		Restore: func(ctx context.Context, archive string) error {
			return backup.RestoreDB(ctx, archive, cfg.Database.DSN)
		},
		Health: HealthCheck(cfg),
		Record: func(ctx context.Context, r store.UpdateRun) {
			db, err := store.Open(ctx, cfg.Database.DSN)
			if err != nil {
				return
			}
			defer db.Close()
			_ = db.AddUpdateRun(ctx, r)
		},
	}
	_ = os.MkdirAll(cfg.DataDir, 0o750)
	return d, cfg, nil
}

// StoredChannel returns the update channel chosen in the application (the setting updates.channel); it is what the server
// itself follows. An unset or unreadable setting gives fallback (the channel of server.yaml).
func StoredChannel(ctx context.Context, pool *pgxpool.Pool, fallback string) string {
	if pool == nil {
		return fallback
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key='updates.channel'`).Scan(&raw); err != nil {
		return fallback
	}
	var v string
	if json.Unmarshal(raw, &v) == nil && (v == update.Beta || v == update.Stable) {
		return v
	}
	return fallback
}

// ActiveChannel is the channel the console and `linager update` use: the one chosen in the application, else server.yaml.
func ActiveChannel(ctx context.Context, cfg *config.Server) string {
	db, err := store.Open(ctx, cfg.Database.DSN)
	if err != nil {
		return cfg.Update.Channel
	}
	defer db.Close()
	return StoredChannel(ctx, db.Pool, cfg.Update.Channel)
}
