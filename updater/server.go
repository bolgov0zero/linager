// Package updater performs the privileged server update: verify, back up, stop, replace,
// migrate, start, health-check and roll back on failure.
package updater

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/release"
	"github.com/bolgov0zero/linager/server/store"
	"github.com/bolgov0zero/linager/update"
)

// Step ids shown in the application.
const (
	StepVerify  = "verify"
	StepBackup  = "backup"
	StepStop    = "stop"
	StepReplace = "replace"
	StepMigrate = "migrate"
	StepStart   = "start"
)

var stepOrder = []string{StepVerify, StepBackup, StepStop, StepReplace, StepMigrate, StepStart}

// Step is one entry of the progress list.
type Step struct {
	ID    string `json:"id"`
	State string `json:"state"` // pending | running | done | failed
	// StartedMs and FinishedMs are unix milliseconds. The panel is served by the very service that is stopped in the
	// middle of an update, so it often learns about the steps only afterwards: the times let it show how long each took.
	StartedMs  int64 `json:"started_ms,omitempty"`
	FinishedMs int64 `json:"finished_ms,omitempty"`
}

// Status is written to the status file after every step; the panel reads it.
type Status struct {
	RunID      string `json:"run_id"`
	State      string `json:"state"` // idle | running | done | failed | rolled_back
	Steps      []Step `json:"steps"`
	From       string `json:"from"`
	To         string `json:"to"`
	Error      string `json:"error,omitempty"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
}

// Request is what the panel asks the root updater to do (file written by the server user).
type Request struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	By      string `json:"by"`
}

var versionRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[A-Za-z0-9.]+)?$`)

// Deps are the collaborators of an update run; tests replace them.
type Deps struct {
	Cfg        *config.Server
	ConfigPath string
	BinPath    string
	CacheDir   string
	PrevDir    string
	// StageDir is a root-only directory where the verified binary is copied before the service stops.
	// Nothing the unprivileged server user can write is trusted after that copy. Empty = PrevDir.
	StageDir string
	// AllowDowngrade permits installing a version that is not newer than the running one (CLI only).
	AllowDowngrade bool
	// Restore puts the database back from a backup archive after a failed update that already ran migrations.
	Restore        func(ctx context.Context, archive string) error
	StatusPath     string
	RequestPath    string
	Source         update.Source
	Key            *release.Key
	CurrentVersion string
	Run            func(ctx context.Context, name string, args ...string) ([]byte, error)
	Backup         func(ctx context.Context) (string, error)
	Health         func(ctx context.Context) error
	HealthTimeout  time.Duration
	Record         func(ctx context.Context, r store.UpdateRun)
	Now            func() time.Time
	OnStatus       func(Status)
}

type runner struct {
	d  Deps
	st Status
}

func (r *runner) now() time.Time {
	if r.d.Now != nil {
		return r.d.Now()
	}
	return time.Now()
}

func (r *runner) save() {
	b, _ := json.Marshal(r.st)
	// The status file lives in a directory the unprivileged server owns: O_EXCL never follows a planted link.
	tmp := r.d.StatusPath + ".tmp"
	_ = os.Remove(tmp)
	if f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err == nil { //nolint:gosec // the unprivileged server reads this file
		_, werr := f.Write(b)
		if cerr := f.Close(); werr == nil && cerr == nil {
			_ = os.Rename(tmp, r.d.StatusPath)
		}
	}
	if r.d.OnStatus != nil {
		r.d.OnStatus(r.st)
	}
}

func (r *runner) set(id, state string) {
	ms := r.now().UnixMilli()
	for i := range r.st.Steps {
		if r.st.Steps[i].ID != id {
			continue
		}
		s := &r.st.Steps[i]
		s.State = state
		switch state {
		case "running":
			s.StartedMs, s.FinishedMs = ms, 0
		case "done", "failed":
			if s.StartedMs == 0 {
				s.StartedMs = ms
			}
			s.FinishedMs = ms
		}
	}
	r.save()
}

// ReadStatus loads the status file (idle when missing).
func ReadStatus(path string) Status {
	b, err := os.ReadFile(path) //nolint:gosec // fixed status path
	if err != nil {
		return Status{State: "idle", Steps: []Step{}}
	}
	var s Status
	if json.Unmarshal(b, &s) != nil {
		return Status{State: "idle", Steps: []Step{}}
	}
	return s
}

// copyFile copies src to dst through a temporary file and a rename. Neither side follows symbolic links,
// and when wantSHA is set the copy is rejected unless its SHA-256 matches.
func copyFile(src, dst string, mode os.FileMode, wantSHA string) error {
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // paths are built by the updater itself
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if st, err := in.Stat(); err != nil || !st.Mode().IsRegular() {
		return errors.New(src + " is not a regular file")
	}
	tmp := dst + ".tmp"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode) //nolint:gosec // see above
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && wantSHA != "" && hex.EncodeToString(h.Sum(nil)) != strings.ToLower(wantSHA) {
		err = errors.New("the binary changed after it was verified")
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// ServerUpdate runs one update to version ("" = latest of the channel). It never touches the system
// before the release is verified and the backup exists.
func ServerUpdate(ctx context.Context, d Deps, version, channel string) (err error) {
	r := &runner{d: d}
	r.st = Status{RunID: fmt.Sprintf("%d", r.now().UnixNano()), State: "running", From: d.CurrentVersion, StartedAt: r.now().Unix()}
	for _, id := range stepOrder {
		r.st.Steps = append(r.st.Steps, Step{ID: id, State: "pending"})
	}
	r.save()
	defer func() {
		r.st.FinishedAt = r.now().Unix()
		switch {
		case err == nil:
			r.st.State = "done"
		case r.st.State != "rolled_back":
			r.st.State = "failed"
		}
		if err != nil {
			r.st.Error = err.Error()
		}
		r.save()
		if d.Record != nil {
			status := map[string]string{"done": "success", "rolled_back": "rolled_back"}[r.st.State]
			if status == "" {
				status = "failed"
			}
			d.Record(context.WithoutCancel(ctx), store.UpdateRun{Kind: "server", FromVersion: d.CurrentVersion, ToVersion: r.st.To, Status: status, Detail: r.st.Error})
		}
	}()

	// 1. verify
	r.set(StepVerify, "running")
	cached, verr := r.prepare(ctx, version, channel)
	if verr != nil {
		r.set(StepVerify, "failed")
		return verr
	}
	r.st.To = cached.Version
	failVerify := func(e error) error { r.set(StepVerify, "failed"); return e }
	switch cmp := release.Compare(release.ParseVersion(cached.Version), release.ParseVersion(d.CurrentVersion)); {
	case cmp == 0 && !d.AllowDowngrade:
		return failVerify(errors.New("already up to date"))
	case cmp < 0 && !d.AllowDowngrade:
		return failVerify(fmt.Errorf("refusing to downgrade from %s to %s", d.CurrentVersion, cached.Version))
	}
	if version == "" && channel != update.Beta && cached.Manifest.Channel == update.Beta {
		return failVerify(errors.New("the release is marked beta in its signed manifest; switch the channel to install it"))
	}
	// Keep a private copy of the verified binary: the release cache is writable by the unprivileged
	// server, so it must not be read again once the checks have passed.
	stageDir := d.StageDir
	if stageDir == "" {
		stageDir = d.PrevDir
	}
	if merr := os.MkdirAll(stageDir, 0o700); merr != nil {
		return failVerify(merr)
	}
	staged := filepath.Join(stageDir, "staged-"+strings.TrimPrefix(cached.Version, "v"))
	if cerr := copyFile(cached.BinaryPath(), staged, 0o755, cached.Asset().SHA256); cerr != nil {
		return failVerify(fmt.Errorf("stage binary: %w", cerr))
	}
	defer func() { _ = os.Remove(staged) }()
	r.set(StepVerify, "done")

	// 2. backup (nothing has been changed yet; a failure here aborts safely)
	r.set(StepBackup, "running")
	dbBackup := ""
	if d.Backup != nil {
		p, berr := d.Backup(ctx)
		if berr != nil {
			r.set(StepBackup, "failed")
			return fmt.Errorf("backup: %w", berr)
		}
		dbBackup = p
	}
	r.set(StepBackup, "done")

	// 3. stop
	r.set(StepStop, "running")
	if out, serr := d.Run(ctx, "systemctl", "stop", "linager-server"); serr != nil {
		r.set(StepStop, "failed")
		return fmt.Errorf("stop service: %s", lastLine(out, serr))
	}
	r.set(StepStop, "done")

	// 4. replace the binary, keeping the previous one for rollback
	r.set(StepReplace, "running")
	prev := filepath.Join(d.PrevDir, "linager-"+strings.TrimPrefix(d.CurrentVersion, "v"))
	_ = os.MkdirAll(d.PrevDir, 0o700)
	if cerr := copyFile(d.BinPath, prev, 0o755, ""); cerr != nil {
		r.set(StepReplace, "failed")
		_, _ = d.Run(ctx, "systemctl", "start", "linager-server")
		return fmt.Errorf("keep previous binary: %w", cerr)
	}
	if cerr := copyFile(staged, d.BinPath, 0o755, cached.Asset().SHA256); cerr != nil {
		r.set(StepReplace, "failed")
		r.rollback(ctx, prev, "")
		return fmt.Errorf("install binary: %w", cerr)
	}
	r.set(StepReplace, "done")

	// 5. migrations (forward only)
	r.set(StepMigrate, "running")
	if out, merr := d.Run(ctx, d.BinPath, "server", "migrate", "--config", d.ConfigPath); merr != nil {
		r.set(StepMigrate, "failed")
		return fmt.Errorf("migrations: %s%s", lastLine(out, merr), r.rollback(ctx, prev, dbBackup))
	}
	r.set(StepMigrate, "done")

	// 6. start and check
	r.set(StepStart, "running")
	if out, serr := d.Run(ctx, "systemctl", "start", "linager-server"); serr != nil {
		r.set(StepStart, "failed")
		return fmt.Errorf("start service: %s%s", lastLine(out, serr), r.rollback(ctx, prev, dbBackup))
	}
	if herr := r.waitHealthy(ctx); herr != nil {
		r.set(StepStart, "failed")
		return fmt.Errorf("the new version did not become healthy: %w%s", herr, r.rollback(ctx, prev, dbBackup))
	}
	r.set(StepStart, "done")
	return nil
}

func lastLine(out []byte, err error) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return err.Error()
}

// prepare returns a verified cached release, downloading it when needed.
func (r *runner) prepare(ctx context.Context, version, channel string) (*update.Cached, error) {
	if version != "" && !versionRe.MatchString(version) {
		return nil, errors.New("invalid version")
	}
	if version != "" {
		if c, err := update.LoadCached(r.d.Key, r.d.CacheDir, version); err == nil {
			return c, nil
		}
	}
	rels, err := r.d.Source.Releases(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read releases: %w", err)
	}
	var rel update.Release
	if version == "" {
		l, ok := update.Latest(rels, channel)
		if !ok {
			return nil, errors.New("no release available")
		}
		rel = l
		if c, err := update.LoadCached(r.d.Key, r.d.CacheDir, rel.Tag); err == nil {
			return c, nil
		}
	} else if rel, err = update.Find(rels, version); err != nil {
		return nil, err
	}
	c, err := update.Download(ctx, rel, r.d.Key, r.d.CacheDir)
	if err == nil {
		r.shareCache()
	}
	return c, err
}

// shareCache gives the release cache to the owner of the data directory: the updater runs as root, but
// the unprivileged server serves the cached releases to agents.
func (r *runner) shareCache() {
	if r.d.Cfg == nil {
		return
	}
	st, err := os.Stat(r.d.Cfg.DataDir)
	if err != nil {
		return
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	_ = filepath.Walk(r.d.CacheDir, func(p string, _ os.FileInfo, err error) error {
		if err == nil {
			_ = os.Lchown(p, int(sys.Uid), int(sys.Gid)) //nolint:gosec // best-effort ownership change of our own cache directory
		}
		return nil
	})
}

// rollback puts the previous binary back. When the migrations of the new version have already run
// (dbBackup is set), the database is restored from the pre-update backup too: migrations are forward
// only, and the previous binary must not run on a newer schema. It returns a note for the error text.
func (r *runner) rollback(ctx context.Context, prev, dbBackup string) (note string) {
	bg := context.WithoutCancel(ctx)
	_, _ = r.d.Run(bg, "systemctl", "stop", "linager-server")
	_ = copyFile(prev, r.d.BinPath, 0o755, "")
	if dbBackup != "" && r.d.Restore != nil {
		if err := r.d.Restore(bg, dbBackup); err != nil {
			note = fmt.Sprintf("; the previous version is back, but the database could not be restored automatically (%v): run `linager restore %s`", err, dbBackup)
		} else {
			note = "; the previous version and the database from " + filepath.Base(dbBackup) + " were restored"
		}
	}
	_, _ = r.d.Run(bg, "systemctl", "start", "linager-server")
	r.st.State = "rolled_back"
	return note
}

func (r *runner) waitHealthy(ctx context.Context) error {
	timeout := r.d.HealthTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for { // always check at least once
		last := r.d.Health(ctx)
		if last == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// ApplyRequest reads and removes the request file, then runs the update.
func ApplyRequest(ctx context.Context, d Deps) error {
	b, err := os.ReadFile(d.RequestPath) //nolint:gosec // fixed request path
	if err != nil {
		return err
	}
	_ = os.Remove(d.RequestPath)
	var req Request
	if err := json.Unmarshal(b, &req); err != nil {
		return err
	}
	if req.Channel != update.Beta {
		req.Channel = update.Stable
	}
	return ServerUpdate(ctx, d, req.Version, req.Channel)
}

// HealthCheck builds the default check: the application port answers (an unauthenticated request of the API gets its 401,
// that is, the server is up and the API is served) and the agent port accepts TCP. The application port is the one the
// interface stored, or app.listen of the file.
func HealthCheck(cfg *config.Server) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if port := appPort(ctx, cfg); port != "" {
			// the certificate is the one of the server itself, asked on this very host
			tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}} //nolint:gosec // loopback health probe
			defer tr.CloseIdleConnections()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+net.JoinHostPort("127.0.0.1", port)+"/api/me", nil)
			resp, err := (&http.Client{Timeout: 5 * time.Second, Transport: tr}).Do(req)
			if err != nil {
				return fmt.Errorf("application port: %w", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("application port answered %s", resp.Status)
			}
		}
		_, ap, err := net.SplitHostPort(cfg.Agents.Listen)
		if err != nil {
			return err
		}
		c, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", ap))
		if err != nil {
			return fmt.Errorf("agent port: %w", err)
		}
		_ = c.Close()
		return nil
	}
}

// appPort is the port of the application: stored by the interface (the setting "connection") or app.listen of the file;
// empty when there is none.
func appPort(ctx context.Context, cfg *config.Server) string {
	port := ""
	if cfg.App.Listen != "" {
		if _, p, err := net.SplitHostPort(cfg.App.Listen); err == nil {
			port = p
		}
	}
	db, err := store.Open(ctx, cfg.Database.DSN)
	if err != nil {
		return port
	}
	defer db.Close()
	if raw := db.SettingRaw(ctx, "connection"); raw != nil {
		var st struct {
			AgentPort int `json:"agent_port"`
			AppPort   int `json:"app_port"`
		}
		if json.Unmarshal(raw, &st) == nil && st.AgentPort > 0 {
			port = ""
			if st.AppPort > 0 {
				port = strconv.Itoa(st.AppPort)
			}
		}
	}
	return port
}
