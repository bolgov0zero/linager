package console

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bolgov0zero/linager/backup"
	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/internal/release"
	"github.com/bolgov0zero/linager/server/store"
	"github.com/bolgov0zero/linager/update"
	"github.com/bolgov0zero/linager/updater"
)

// Service is the state of a systemd unit.
type Service struct {
	State string    // active, inactive, failed, activating, deactivating, not-found or unknown
	Since time.Time // when it became active
}

// ServerStatus is what the menu shows about the server. A number of -1 means "could not be read".
type ServerStatus struct {
	Service     Service
	AppPort     string // the port of the application ("" when it is not set)
	AgentListen string
	DBOK        bool
	DBErr       string
	Clients     int
	Pending     int
	Users       int
	Access      string // off, white or black
	LastBackup  time.Time
	NeedRoot    bool // the configuration is closed to this user
	ConfigErr   string
}

// AgentStatus is what the menu shows about the agent.
type AgentStatus struct {
	Service    Service
	Enrolled   bool
	Server     string
	ModulesOn  int
	ModulesAll int
	NeedRoot   bool
}

// Probe collects the state. The zero value is not usable: build it with NewProbe.
type Probe struct {
	Layout  Layout
	Version string
	// Exec runs a command and returns its standard output; tests replace it.
	Exec func(ctx context.Context, name string, args ...string) (string, error)

	mu   sync.Mutex
	pool *pgxpool.Pool
	dsn  string
}

// NewProbe returns a probe for the real system.
func NewProbe(l Layout, version string) *Probe {
	return &Probe{Layout: l, Version: version, Exec: func(ctx context.Context, name string, args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // fixed commands
		return string(out), err
	}}
}

// Close releases the database connection.
func (p *Probe) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pool != nil {
		p.pool.Close()
		p.pool = nil
	}
}

func (p *Probe) service(ctx context.Context, unit string) Service {
	out, err := p.Exec(ctx, "systemctl", "show", unit, "-p", "ActiveState", "-p", "LoadState", "-p", "ActiveEnterTimestamp", "--timestamp=unix")
	if err != nil && out == "" {
		return Service{State: "unknown"}
	}
	var s Service
	load := ""
	for _, l := range strings.Split(out, "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(l), "=")
		switch k {
		case "ActiveState":
			s.State = v
		case "LoadState":
			load = v
		case "ActiveEnterTimestamp":
			if n, err := strconv.ParseInt(strings.TrimPrefix(v, "@"), 10, 64); err == nil && n > 0 {
				s.Since = time.Unix(n, 0)
			}
		}
	}
	if load == "not-found" {
		s.State = "not-found"
	}
	if s.State == "" {
		s.State = "unknown"
	}
	return s
}

// db returns a connection pool for dsn, reusing the previous one.
func (p *Probe) db(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pool != nil && p.dsn == dsn {
		return p.pool, nil
	}
	if p.pool != nil {
		p.pool.Close()
		p.pool = nil
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.ConnectTimeout = 2 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	p.pool, p.dsn = pool, dsn
	return pool, nil
}

func (p *Probe) dropDB() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pool != nil {
		p.pool.Close()
		p.pool = nil
	}
}

const statQuery = `SELECT (SELECT count(*) FROM servers),
	(SELECT count(*) FROM enrollments WHERE status='pending' AND expires_at > now()),
	(SELECT count(*) FROM users),
	coalesce((SELECT value->>'mode' FROM settings WHERE key='app_access'), 'off'),
	coalesce((SELECT value->>'app_port' FROM settings WHERE key='connection'), '')`

// Server reads the state of the server; every part that cannot be read stays empty instead of failing the rest.
func (p *Probe) Server(ctx context.Context) *ServerStatus {
	st := &ServerStatus{Clients: -1, Pending: -1, Users: -1}
	st.Service = p.service(ctx, "linager-server")
	if _, t, err := backup.Last(backup.DefaultDir, "linager-backup"); err == nil {
		st.LastBackup = t
	}
	cfg, err := config.LoadServer(p.Layout.ServerConfig)
	if err != nil {
		st.NeedRoot = errors.Is(err, fs.ErrPermission)
		st.ConfigErr = err.Error()
		return st
	}
	st.AgentListen = cfg.Agents.Listen

	qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := p.db(qctx, cfg.Database.DSN)
	if err == nil {
		err = pool.QueryRow(qctx, statQuery).Scan(&st.Clients, &st.Pending, &st.Users, &st.Access, &st.AppPort)
	}
	if err != nil {
		p.dropDB()
		st.DBErr = shortErr(err)
		return st
	}
	st.DBOK = true
	if st.Access == "" {
		st.Access = "off"
	}
	if st.AppPort == "" && cfg.App.Listen != "" {
		if _, p, err := net.SplitHostPort(cfg.App.Listen); err == nil {
			st.AppPort = p
		}
	}
	if st.AppPort == "0" {
		st.AppPort = ""
	}
	return st
}

func shortErr(err error) string {
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return s
}

// Agent reads the state of the agent.
func (p *Probe) Agent(ctx context.Context) *AgentStatus {
	st := &AgentStatus{ModulesAll: len(config.AgentModules)}
	st.Service = p.service(ctx, "linager-agent")
	b, err := os.ReadFile(filepath.Join(p.Layout.AgentDir, "state.json")) //nolint:gosec // fixed name inside the data dir
	switch {
	case err == nil:
		var s struct {
			Server string `json:"server"`
		}
		if json.Unmarshal(b, &s) == nil {
			st.Enrolled, st.Server = s.Server != "", s.Server
		}
	case errors.Is(err, fs.ErrPermission):
		st.NeedRoot = true
	}
	if c, err := config.LoadAgent(p.Layout.AgentConfig); err == nil {
		st.ModulesOn = len(c.List())
	} else {
		st.ModulesOn = -1
	}
	return st
}

// Latest returns the newest release tag when it is newer than this binary, otherwise an empty string.
func (p *Probe) Latest(ctx context.Context) string {
	d, cfg, err := updater.DefaultDeps(p.Layout.ServerConfig)
	if err != nil {
		return ""
	}
	rels, err := d.Source.Releases(ctx)
	if err != nil {
		return ""
	}
	latest, ok := update.Latest(rels, cfg.Update.Channel)
	if ok && release.Newer(latest.Tag, p.Version) {
		return latest.Tag
	}
	return ""
}

// Audit records an administrator action made through the menu; a failure is ignored on purpose.
func (p *Probe) Audit(ctx context.Context, action string, ok bool) {
	cfg, err := config.LoadServer(p.Layout.ServerConfig)
	if err != nil {
		return
	}
	pool, err := p.db(ctx, cfg.Database.DSN)
	if err != nil {
		return
	}
	res := "ok"
	if !ok {
		res = "error"
	}
	_ = (&store.Store{Pool: pool}).WriteAudit(ctx, store.Audit{Actor: "cli", Action: action, TargetType: "server", Result: res, Details: map[string]any{"via": "menu"}})
}

// Fingerprint returns the fingerprint of the certificate on the client port and the port address.
func (p *Probe) Fingerprint() (fp, listen string, err error) {
	cfg, err := config.LoadServer(p.Layout.ServerConfig)
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(filepath.Join(cfg.DataDir, "agents.crt")) //nolint:gosec // fixed name inside the data dir
	if err != nil {
		return "", "", err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return "", "", fmt.Errorf("agents.crt: no certificate")
	}
	return pki.Fingerprint(blk.Bytes), cfg.Agents.Listen, nil
}
