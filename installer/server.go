package installer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

// ServerOpts are values given by flags; zero values are asked interactively.
type ServerOpts struct {
	AgentPort, AppPort int
	Domain, AdminUser  string // Domain: an optional DNS name of the server for the certificate
	PasswordFile       string
	ExtraHosts         []string
	ConfigPath         string
	LogPath            string
	Force              bool
}

type step struct {
	title string
	info  string
	fn    func(ctx context.Context) error
}

// runSteps declares and executes steps, emitting events. It stops at the first failure.
func runSteps(ctx context.Context, c Console, steps []step) error {
	titles := make([]string, len(steps))
	for i, s := range steps {
		titles[i] = s.title
	}
	c.Steps(titles)
	for i, s := range steps {
		c.Event(Event{Index: i, State: StepStart, Info: s.info})
		if err := s.fn(ctx); err != nil {
			var ce *CmdError
			var tail []string
			if errors.As(err, &ce) {
				tail = ce.Tail
			}
			c.Event(Event{Index: i, State: StepFail, Err: err, Tail: tail})
			return err
		}
		c.Event(Event{Index: i, State: StepDone})
	}
	return nil
}

func checkSystem(l Lang) error {
	if err := RequireRoot(); err != nil {
		return errors.New(tr(l, "err.root"))
	}
	b, _ := os.ReadFile("/etc/os-release")
	if info := ParseOSRelease(string(b)); !info.Supported() {
		name := info.Pretty
		if name == "" {
			name = "unknown"
		}
		return fmt.Errorf("%s", tr(l, "err.os", name))
	}
	if !FileExists("/run/systemd/system") {
		return errors.New(tr(l, "err.systemd"))
	}
	return nil
}

func portErr(l Lang, v string) error {
	n, ok := ValidPort(v)
	if !ok {
		return errors.New(tr(l, "err.port"))
	}
	if !PortFree(n) {
		return errors.New(tr(l, "err.portbusy", n))
	}
	return nil
}

// collectServerParams merges flags and interactive answers.
func collectServerParams(c Console, o ServerOpts) (*ServerParams, error) {
	l := c.Lang()
	p := &ServerParams{AgentPort: o.AgentPort, AppPort: o.AppPort,
		Domain: o.Domain, AdminUser: o.AdminUser, ExtraHosts: o.ExtraHosts, ConfigPath: o.ConfigPath, DataDir: DataDir, Force: o.Force}
	if o.PasswordFile != "" {
		b, err := os.ReadFile(o.PasswordFile) //nolint:gosec // operator-supplied path
		if err != nil {
			return nil, err
		}
		p.Password = strings.TrimRight(string(b), "\r\n")
	}
	known := map[string]string{}
	set := func(k, v string) {
		if v != "" {
			known[k] = v
		}
	}
	if o.AgentPort > 0 {
		set("agentport", fmt.Sprint(o.AgentPort))
	}
	if o.AppPort > 0 {
		set("appport", fmt.Sprint(o.AppPort))
	}
	set("user", o.AdminUser)
	need := func(k string) bool { _, ok := known[k]; return !ok }
	form := Form{Title: tr(l, "srv.title"), Fields: []Field{
		{Key: "agentport", Label: tr(l, "srv.agentport"), Hint: tr(l, "srv.agentport.h"), Kind: KindNumber, Required: true,
			Visible:  func(map[string]string) bool { return need("agentport") },
			Validate: func(v string, _ map[string]string) error { return portErr(l, v) }},
		{Key: "appport", Label: tr(l, "srv.appport"), Hint: tr(l, "srv.appport.h"), Kind: KindNumber, Required: true,
			Visible: func(map[string]string) bool { return need("appport") },
			Validate: func(v string, a map[string]string) error {
				if err := portErr(l, v); err != nil {
					return err
				}
				if v == a["agentport"] || v == known["agentport"] {
					return errors.New(tr(l, "err.portsame"))
				}
				return nil
			}},
		{Key: "user", Label: tr(l, "srv.user"), Kind: KindText, Default: "admin", Required: true,
			Visible: func(map[string]string) bool { return need("user") },
			Validate: func(v string, _ map[string]string) error {
				if !ValidLogin(v) {
					return errors.New(tr(l, "err.login"))
				}
				return nil
			}},
		{Key: "password", Label: tr(l, "srv.password"), Hint: tr(l, "srv.password.h"), Kind: KindSecret, Confirm: true,
			Visible: func(map[string]string) bool { return o.PasswordFile == "" },
			Validate: func(v string, _ map[string]string) error {
				if v != "" && len(v) < 12 {
					return errors.New(tr(l, "err.password"))
				}
				return nil
			}},
	}}
	ans := map[string]string{}
	if c.Interactive() {
		a, err := c.Ask(form)
		if err != nil {
			return nil, err
		}
		ans = a
	}
	get := func(k string) string {
		if v, ok := known[k]; ok {
			return v
		}
		return ans[k]
	}
	if !c.Interactive() {
		var missing []string
		if get("agentport") == "" {
			missing = append(missing, "--agent-port")
		}
		if get("appport") == "" {
			missing = append(missing, "--app-port")
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("%s", tr(l, "err.noninteractive", strings.Join(missing, ", ")))
		}
	}
	p.AgentPort, _ = ValidPort(get("agentport"))
	p.AppPort, _ = ValidPort(get("appport"))
	if p.Domain != "" && !ValidDomain(p.Domain) {
		return nil, errors.New(tr(l, "err.domain"))
	}
	if p.AgentPort == 0 || p.AppPort == 0 {
		return nil, errors.New(tr(l, "err.port"))
	}
	if p.AppPort == p.AgentPort {
		return nil, errors.New(tr(l, "err.portsame"))
	}
	if !PortFree(p.AppPort) {
		return nil, errors.New(tr(l, "err.portbusy", p.AppPort))
	}
	if !PortFree(p.AgentPort) {
		return nil, errors.New(tr(l, "err.portbusy", p.AgentPort))
	}
	p.AdminUser = get("user")
	if p.AdminUser == "" {
		p.AdminUser = "admin"
	}
	if !ValidLogin(p.AdminUser) {
		return nil, errors.New(tr(l, "err.login"))
	}
	if p.Password == "" {
		p.Password = ans["password"]
	}
	if p.Password == "" {
		p.Password, p.PasswordGen = RandomString(20), true
	} else if len(p.Password) < 12 {
		return nil, errors.New(tr(l, "err.password"))
	}
	return p, nil
}

// ServerFlow installs and starts the Linager server on this host.
func ServerFlow(o ServerOpts) Flow {
	return func(ctx context.Context, c Console) (*Summary, error) {
		l := c.Lang()
		if o.ConfigPath == "" {
			o.ConfigPath = ConfigFile
		}
		if o.LogPath == "" {
			o.LogPath = LogFile
		}
		if err := checkSystem(l); err != nil {
			return nil, err
		}
		if FileExists(o.ConfigPath) && !o.Force {
			return &Summary{Title: tr(l, "fail.title"), Notes: []string{tr(l, "note.already", o.ConfigPath)}}, errors.New(tr(l, "note.already", o.ConfigPath))
		}
		if o.Force && FileExists(o.ConfigPath) {
			// a reinstall over a running instance must not collide with its own ports
			_ = exec.Command("systemctl", "stop", "linager-server").Run()
		}
		p, err := collectServerParams(c, o)
		if err != nil {
			return nil, err
		}
		run, closeLog := NewRunner(o.LogPath)
		defer closeLog()
		run.Logf("=== linager install server %s agent-port=%d app-port=%d", time.Now().Format(time.RFC3339), p.AgentPort, p.AppPort)

		var fp string
		var hosts []string
		steps := []step{
			{title: tr(l, "st.check"), fn: func(context.Context) error { return checkSystem(l) }},
			{title: tr(l, "st.packages"), info: tr(l, "info.apt"), fn: func(ctx context.Context) error { return installPostgres(ctx, run) }},
			{title: tr(l, "st.database"), fn: func(ctx context.Context) error { return setupDatabase(ctx, run, l, p) }},
			{title: tr(l, "st.config"), fn: func(ctx context.Context) error {
				hosts = LocalHosts(append([]string{p.Domain}, p.ExtraHosts...))
				return writeServerFiles(ctx, run, p, hosts)
			}},
			{title: tr(l, "st.admin"), fn: func(ctx context.Context) error { return createAdmin(ctx, p) }},
		}
		steps = append(steps,
			step{title: tr(l, "st.service"), fn: func(ctx context.Context) error {
				if err := os.WriteFile("/etc/systemd/system/linager-server.service", []byte(ServerUnit), 0o644); err != nil { //nolint:gosec // unit files are world-readable
					return err
				}
				for name, body := range map[string]string{"linager-updater.path": UpdaterPathUnit, "linager-updater.service": UpdaterServiceUnit,
					"linager-backup.service": BackupServiceUnit, "linager-backup.timer": BackupTimerUnit} {
					if err := os.WriteFile("/etc/systemd/system/"+name, []byte(body), 0o644); err != nil { //nolint:gosec // unit files are world-readable
						return err
					}
				}
				if err := run.Run(ctx, "", "systemctl", "daemon-reload"); err != nil {
					return err
				}
				if err := run.Run(ctx, "", "systemctl", "enable", "--now", "linager-updater.path", "linager-backup.timer"); err != nil {
					return err
				}
				if err := run.Run(ctx, "", "systemctl", "enable", "linager-server"); err != nil {
					return err
				}
				return run.Run(ctx, "", "systemctl", "restart", "linager-server")
			}},
			step{title: tr(l, "st.verify"), info: tr(l, "info.wait"), fn: func(ctx context.Context) error {
				var err error
				fp, err = verifyServer(ctx, p)
				return err
			}},
		)
		if err := runSteps(ctx, c, steps); err != nil {
			return nil, err
		}
		return serverSummary(l, p, hosts, fp, o.LogPath), nil
	}
}

func installPostgres(ctx context.Context, run *Runner) error {
	apt := []string{"-o", "DPkg::Lock::Timeout=180", "-y", "-q"}
	if err := run.Run(ctx, "", "apt-get", append(apt, "update")...); err != nil {
		return err
	}
	if err := run.Run(ctx, "", "apt-get", append(apt, "install", "postgresql", "ca-certificates")...); err != nil {
		return err
	}
	if err := run.Run(ctx, "", "systemctl", "enable", "--now", "postgresql"); err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		if run.Run(ctx, "", "runuser", "-u", "postgres", "--", "pg_isready", "-q") == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("PostgreSQL did not become ready")
}

func psql(ctx context.Context, run *Runner, sql string, db ...string) (string, error) {
	args := []string{"-u", "postgres", "--", "psql", "-v", "ON_ERROR_STOP=1", "-At"}
	if len(db) > 0 {
		args = append(args, "-d", db[0])
	}
	args = append(args, "-c", sql)
	out, err := run.Output(ctx, "", "runuser", args...)
	return strings.TrimSpace(out), err
}

func setupDatabase(ctx context.Context, run *Runner, l Lang, p *ServerParams) error {
	listen, err := psql(ctx, run, "SHOW listen_addresses")
	if err != nil {
		return err
	}
	for _, a := range strings.Split(listen, ",") {
		a = strings.TrimSpace(a)
		if a == "*" || a == "0.0.0.0" || a == "::" {
			return fmt.Errorf("%s", tr(l, "err.pglisten", listen))
		}
	}
	bad, err := psql(ctx, run, "SELECT count(*) FROM pg_hba_file_rules WHERE type LIKE 'host%' AND auth_method IN ('trust','md5','password')")
	if err != nil {
		return err
	}
	if bad != "0" {
		return errors.New(tr(l, "err.pghba"))
	}
	port, err := psql(ctx, run, "SHOW port")
	if err != nil {
		return err
	}
	pw := RandomString(32)
	exists, err := psql(ctx, run, "SELECT 1 FROM pg_roles WHERE rolname='linager'")
	if err != nil {
		return err
	}
	verb := "CREATE"
	if exists == "1" {
		verb = "ALTER"
	}
	if _, err := psql(ctx, run, fmt.Sprintf("SET password_encryption='scram-sha-256'; %s ROLE linager LOGIN PASSWORD '%s'", verb, pw)); err != nil {
		return err
	}
	if has, err := psql(ctx, run, "SELECT 1 FROM pg_database WHERE datname='linager'"); err != nil {
		return err
	} else if has != "1" {
		if _, err := psql(ctx, run, "CREATE DATABASE linager OWNER linager"); err != nil {
			return err
		}
	}
	p.DSN = fmt.Sprintf("postgres://linager:%s@127.0.0.1:%s/linager?sslmode=disable", pw, port)
	return nil
}

func installSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if a, err1 := filepath.EvalSymlinks(exe); err1 == nil {
		exe = a
	}
	if b, err1 := filepath.EvalSymlinks(BinPath); err1 == nil && b == exe {
		return nil
	}
	src, err := os.Open(exe) //nolint:gosec // our own executable
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	tmp := BinPath + ".new"
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) //nolint:gosec // executable must be 0755
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, BinPath)
}

func writeServerFiles(ctx context.Context, run *Runner, p *ServerParams, hosts []string) error {
	if err := installSelf(); err != nil {
		return err
	}
	if run.Run(ctx, "", "id", "linager") != nil {
		if err := run.Run(ctx, "", "useradd", "--system", "--home-dir", p.DataDir, "--shell", "/usr/sbin/nologin", "linager"); err != nil {
			return err
		}
	}
	if old, err := config.LoadServer(p.ConfigPath); err == nil { // keep the update settings of an existing install
		p.UpdateSource, p.UpdateChannel = old.Update.Source, old.Update.Channel
	}
	if err := os.MkdirAll(filepath.Dir(p.ConfigPath), 0o755); err != nil { //nolint:gosec // config dir is traversable, the file itself is 0600
		return err
	}
	if err := os.WriteFile(p.ConfigPath, []byte(RenderServerConfig(p, hosts)), 0o600); err != nil {
		return err
	}
	if err := server.InitKeys(p.DataDir, hosts); err != nil {
		return err
	}
	if err := run.Run(ctx, "", "chown", "-R", "linager:linager", p.DataDir); err != nil {
		return err
	}
	return run.Run(ctx, "", "chown", "linager:linager", p.ConfigPath)
}

func createAdmin(ctx context.Context, p *ServerParams) error {
	db, err := store.Open(ctx, p.DSN) // also applies migrations
	if err != nil {
		return err
	}
	defer db.Close()
	if n, err := db.UserCount(ctx); err != nil {
		return err
	} else if n > 0 {
		if p.Force {
			p.PasswordGen = false // existing administrator is kept untouched
			p.Password = ""
			// the ports of the wizard win over the ones the panel stored earlier
			return db.DeleteSetting(ctx, "connection")
		}
		return errors.New("an administrator already exists")
	}
	return server.CreateAdmin(ctx, db, p.AdminUser, p.Password)
}

func verifyServer(ctx context.Context, p *ServerParams) (string, error) {
	// the application port answers: an unauthenticated request of the API gets its 401
	addr := fmt.Sprintf("127.0.0.1:%d", p.AppPort)
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}} //nolint:gosec // loopback probe of our own certificate
	defer tr.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: tr}
	deadline := time.Now().Add(40 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/api/me", nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				break
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("the application port did not respond at %s (see journalctl -u linager-server)", addr)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	conn, err := (&tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}}). //nolint:gosec // only reads the certificate to compute its fingerprint
																					DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", p.AgentPort))
	if err != nil {
		return "", fmt.Errorf("agent port %d does not answer: %w", p.AgentPort, err)
	}
	defer func() { _ = conn.Close() }()
	cs := conn.(*tls.Conn).ConnectionState()
	if len(cs.PeerCertificates) == 0 {
		return "", errors.New("agent port presented no certificate")
	}
	return pki.Fingerprint(cs.PeerCertificates[0].Raw), nil
}

func primaryHost(hosts []string) string {
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil && ip.To4() != nil {
			return h
		}
	}
	if len(hosts) > 0 {
		return hosts[0]
	}
	return "SERVER"
}

func serverSummary(l Lang, p *ServerParams, hosts []string, fp, logPath string) *Summary {
	host := primaryHost(hosts)
	s := &Summary{Title: tr(l, "done.server"), Success: true}
	s.Lines = append(s.Lines, KV{Label: tr(l, "sum.admin"), Value: fmt.Sprintf("%s:%d", host, p.AppPort), Mono: true}, KV{Label: tr(l, "sum.login"), Value: p.AdminUser, Mono: true})
	if p.PasswordGen {
		s.Lines = append(s.Lines, KV{Label: tr(l, "sum.password"), Value: p.Password, Mono: true})
		s.Notes = append(s.Notes, tr(l, "sum.password")+": "+tr(l, "sum.password.gen"))
	}
	s.Lines = append(s.Lines,
		KV{Label: tr(l, "sum.agentport"), Value: fmt.Sprint(p.AgentPort)},
		KV{Label: tr(l, "sum.appport"), Value: fmt.Sprint(p.AppPort)},
		KV{Label: tr(l, "sum.fp"), Value: fp, Mono: true},
		KV{Label: tr(l, "sum.client"), Value: fmt.Sprintf("linager install agent --server %s:%d --fingerprint %s", host, p.AgentPort, fp), Mono: true},
		KV{Label: tr(l, "sum.log"), Value: logPath},
	)
	s.Notes = append(s.Notes, tr(l, "note.app", fmt.Sprintf("%s:%d", host, p.AppPort)))
	if exec := (&Runner{log: io.Discard}); exec.Run(context.Background(), "", "systemctl", "is-active", "--quiet", "nftables") == nil {
		s.Notes = append(s.Notes, tr(l, "note.nft", p.AgentPort, p.AppPort))
	}
	return s
}
