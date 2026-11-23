package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/bolgov0zero/linager/internal/release"
)

// The install over SSH: the panel opens an SSH connection to a new host, checks it, and runs the same install as
// the link does. Everything that is secret (the password, the key, the sudo password) lives only in the memory of
// a session, which ends with the install, with a cancel or after sshIdle without a call.
const (
	sshIdle      = 15 * time.Minute
	sshMaxLog    = 400
	sshMaxOutput = 1 << 20
	sshTokenTTL  = 10 * time.Minute
)

// How long the install waits for the server to take the agent and for the agent to connect; tests shorten them.
var (
	sshBindMax    = 30 * time.Second
	sshConnectMax = 60 * time.Second
)

// Stages. The first eight are the checks, the next seven the install.
var (
	sshCheckIDs   = []string{"connect", "hostkey", "auth", "system", "disk", "rights", "reach", "agent"}
	sshInstallIDs = []string{"token", "download", "verify", "install", "bind", "online"}
)

type sshStep struct {
	ID         string `json:"id"`
	State      string `json:"state"` // pending, running, done, warn, failed
	StartedMS  int64  `json:"started_ms,omitempty"`
	FinishedMS int64  `json:"finished_ms,omitempty"`
	// Detail is a fact (a system name, an address, an error text); Code names a failure so the panel can explain it.
	Detail string `json:"detail,omitempty"`
	Code   string `json:"code,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

type sshGate struct {
	Kind string         `json:"kind"` // hostkey, sudo, address, reinstall
	Data map[string]any `json:"data,omitempty"`
}

type gateAnswer struct {
	Accept   bool
	Password string
	Address  string
}

// SSHStart is what a person fills in.
type SSHStart struct {
	Host       string
	Port       int
	Login      string
	Auth       string // password or key
	Password   string
	Key        string
	Passphrase string
	GroupID    *int64
	// PanelHost is the address the administrator opened the panel with: one more address the host may try.
	PanelHost string
}

// SSHDeps is everything the installer needs from the rest of the server; tests replace it.
type SSHDeps struct {
	// Hosts lists the addresses (names or IPs, no port) at which a new host may reach this server.
	Hosts       func() []string
	Port        string
	Fingerprint string
	Pin         string
	// Assets returns the binaries of the release that agents get, by architecture.
	Assets func(ctx context.Context) (map[string]release.Asset, error)
	// NewToken makes the one-shot token of an install; EndToken turns it off.
	NewToken func(ctx context.Context, by string, group *int64) (secret string, id int64, err error)
	EndToken func(ctx context.Context, id int64)
	// Observe tells whether the agent that used the token has been enrolled, and whether it is connected.
	Observe func(ctx context.Context, tokenID int64) (serverID, name string, bound, online bool)
	// Forget removes a server that was created by an install that then failed.
	Forget func(ctx context.Context, serverID string)
	// Audit records the end of a check or an install; it never gets a secret.
	Audit func(ctx context.Context, user, action, target, result string, details map[string]any)
}

// SSHInstaller owns the sessions.
type SSHInstaller struct {
	portMu sync.RWMutex // guards deps.Port, which changes when the agent port is moved
	deps   SSHDeps
	// deny says which target addresses are refused; allowed, when not empty, limits them to some networks.
	deny    func(netip.Addr) bool
	allowed []netip.Prefix
	// lookup resolves a host name (tests replace it).
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)

	mu       sync.Mutex
	sessions map[string]*sshSession
}

// NewSSHInstaller builds the installer; allowed is the list of networks from the configuration.
func NewSSHInstaller(deps SSHDeps, allowed []string) *SSHInstaller {
	in := &SSHInstaller{deps: deps, deny: deniedTarget, sessions: map[string]*sshSession{}, lookup: lookupHost}
	for _, n := range allowed {
		if p, err := netip.ParsePrefix(n); err == nil {
			in.allowed = append(in.allowed, p)
		}
	}
	return in
}

// deniedTarget refuses the addresses that make the panel a tool against itself or against the cloud metadata
// service. Private networks stay allowed: that is where most hosts are.
func deniedTarget(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsValid() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

func lookupHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	return names, err
}

// ErrSSHTarget is returned for an address that may not be used.
var ErrSSHTarget = errors.New("this address is not allowed")

// target resolves host and returns the first address that may be used.
func (in *SSHInstaller) target(ctx context.Context, host string) (netip.Addr, error) {
	ips, err := in.lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return netip.Addr{}, fmt.Errorf("cannot resolve %s", host)
	}
	for _, ip := range ips {
		if in.deny(ip) {
			return netip.Addr{}, ErrSSHTarget
		}
	}
	ip := ips[0].Unmap()
	if len(in.allowed) > 0 {
		ok := false
		for _, p := range in.allowed {
			if p.Contains(ip) {
				ok = true
			}
		}
		if !ok {
			return netip.Addr{}, ErrSSHTarget
		}
	}
	return ip, nil
}

type sshSession struct {
	id    string
	owner int64
	login string // of the administrator, for the audit

	in  *SSHInstaller
	ctx context.Context
	end context.CancelFunc

	mu       sync.Mutex
	last     time.Time
	host     string
	ip       netip.Addr
	port     int
	user     string
	auth     string
	secret   string
	phrase   string
	sudoPW   string
	groupID  *int64
	phase    string
	steps    []sshStep
	gate     *sshGate
	gateCh   chan gateAnswer
	errText  string
	log      []string
	logTotal int
	server   string // host:port that the new agent will use
	mode     string // root, nopass or password
	arch     string
	hasWget  bool
	reinst   bool
	hostFP   string
	extra    string
	client   *ssh.Client
	tokenID  int64
	mask     string
	newID    string
	newName  string
	running  bool
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Start opens a session and begins the checks in the background.
func (in *SSHInstaller) Start(owner int64, login string, req SSHStart) (*sshSession, error) {
	ip, err := in.target(context.Background(), req.Host)
	if err != nil {
		return nil, err
	}
	in.mu.Lock()
	live := 0
	for _, s := range in.sessions {
		if s.owner == owner {
			live++
		}
	}
	if live >= 3 {
		in.mu.Unlock()
		return nil, errors.New("too many open installs; finish or cancel one")
	}
	ctx, end := context.WithCancel(context.Background())
	s := &sshSession{id: newID(), owner: owner, login: login, in: in, ctx: ctx, end: end, last: time.Now(), host: req.Host, ip: ip, port: req.Port,
		user: req.Login, auth: req.Auth, secret: req.Password, phase: "checking", gateCh: make(chan gateAnswer, 1), groupID: req.GroupID, extra: req.PanelHost}
	if req.Auth == "key" {
		s.secret, s.phrase = req.Key, req.Passphrase
	}
	s.steps = newSSHSteps(sshCheckIDs)
	in.sessions[s.id] = s
	in.mu.Unlock()
	go s.runCheck()
	return s, nil
}

// Get returns a session of its owner.
func (in *SSHInstaller) Get(id string, owner int64) (*sshSession, bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	s, ok := in.sessions[id]
	if !ok || s.owner != owner {
		return nil, false
	}
	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()
	return s, true
}

// Close ends a session and wipes its secrets.
func (in *SSHInstaller) Close(id string, owner int64) bool {
	in.mu.Lock()
	s, ok := in.sessions[id]
	if ok && s.owner == owner {
		delete(in.sessions, id)
	} else {
		ok = false
	}
	in.mu.Unlock()
	if ok {
		s.wipe()
	}
	return ok
}

// Run removes the sessions nobody has asked about for a while.
func (in *SSHInstaller) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			in.mu.Lock()
			var old []*sshSession
			for id, s := range in.sessions {
				s.mu.Lock()
				idle := !s.running && time.Since(s.last) > sshIdle
				s.mu.Unlock()
				if idle {
					old = append(old, s)
					delete(in.sessions, id)
				}
			}
			in.mu.Unlock()
			for _, s := range old {
				s.wipe()
			}
		}
	}
}

// wipe stops the session and forgets every secret.
func (s *sshSession) wipe() {
	s.end()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secret, s.phrase, s.sudoPW = "", "", ""
	if s.client != nil {
		_ = s.client.Close()
		s.client = nil
	}
	if s.phase == "checking" || s.phase == "ready" {
		s.phase = "closed"
	}
}

func newSSHSteps(ids []string) []sshStep {
	out := make([]sshStep, len(ids))
	for i, id := range ids {
		out[i] = sshStep{ID: id, State: "pending"}
	}
	return out
}

// ---- state for the panel ----

// SSHState is what the panel polls.
type SSHState struct {
	ID       string    `json:"id"`
	Phase    string    `json:"phase"` // checking, ready, check_failed, installing, done, install_failed, closed
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Login    string    `json:"login"`
	Steps    []sshStep `json:"steps"`
	Gate     *sshGate  `json:"gate,omitempty"`
	Error    string    `json:"error,omitempty"`
	Server   string    `json:"server,omitempty"`
	Mode     string    `json:"mode,omitempty"`
	Log      []string  `json:"log"`
	LogTotal int       `json:"log_total"`
	NewID    string    `json:"new_id,omitempty"`
	NewName  string    `json:"new_name,omitempty"`
	NowMS    int64     `json:"now_ms"`
}

// State returns a copy for the panel.
func (s *sshSession) State() SSHState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := SSHState{ID: s.id, Phase: s.phase, Host: s.host, Port: s.port, Login: s.user, Steps: append([]sshStep(nil), s.steps...), Error: s.errText,
		Server: s.server, Mode: s.mode, Log: append([]string(nil), s.log...), LogTotal: s.logTotal, NewID: s.newID, NewName: s.newName, NowMS: time.Now().UnixMilli()}
	if s.gate != nil {
		g := *s.gate
		st.Gate = &g
	}
	return st
}

func (s *sshSession) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addLog(line)
}

// addLog stores a line with the token masked; the caller holds the lock.
func (s *sshSession) addLog(line string) {
	if s.mask != "" {
		line = strings.ReplaceAll(line, s.mask, "••••")
	}
	if s.sudoPW != "" {
		line = strings.ReplaceAll(line, s.sudoPW, "••••")
	}
	s.logTotal++
	s.log = append(s.log, line)
	if len(s.log) > sshMaxLog {
		s.log = s.log[len(s.log)-sshMaxLog:]
	}
}

func (s *sshSession) setStep(i int, state, detail, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.steps[i]
	now := time.Now().UnixMilli()
	if state == "running" && st.StartedMS == 0 {
		st.StartedMS = now
	}
	if state == "done" || state == "failed" || state == "warn" {
		if st.StartedMS == 0 {
			st.StartedMS = now
		}
		st.FinishedMS = now
	}
	st.State = state
	if detail != "" || state == "done" {
		st.Detail = detail
	}
	st.Code = code
}

func (s *sshSession) setProgress(i int, n, size int64) {
	s.mu.Lock()
	s.steps[i].Bytes, s.steps[i].Size = n, size
	s.mu.Unlock()
}

// ---- the gates: a question to the administrator in the middle of a run ----

var errGateClosed = errors.New("the install was cancelled")

func (s *sshSession) askGate(kind string, data map[string]any) (gateAnswer, error) {
	s.mu.Lock()
	s.gate = &sshGate{Kind: kind, Data: data}
	s.mu.Unlock()
	select {
	case a := <-s.gateCh:
		s.mu.Lock()
		s.gate = nil
		s.mu.Unlock()
		return a, nil
	case <-s.ctx.Done():
		return gateAnswer{}, errGateClosed
	}
}

// Answer hands the answer of the administrator to the run that waits for it.
func (s *sshSession) Answer(kind string, a gateAnswer) error {
	s.mu.Lock()
	g := s.gate
	s.mu.Unlock()
	if g == nil || g.Kind != kind {
		return errors.New("nothing is waiting for this answer")
	}
	select {
	case s.gateCh <- a:
		return nil
	default:
		return errors.New("an answer was already given")
	}
}

// ---- the connection ----

var errHostKeyRejected = errors.New("the host key was not accepted")

func (s *sshSession) authMethods() ([]ssh.AuthMethod, error) {
	s.mu.Lock()
	auth, secret, phrase := s.auth, s.secret, s.phrase
	s.mu.Unlock()
	if auth == "key" {
		signer, err := ssh.ParsePrivateKey([]byte(secret))
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(secret), []byte(phrase))
		}
		if err != nil {
			return nil, fmt.Errorf("the private key cannot be read: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	return []ssh.AuthMethod{
		ssh.Password(secret),
		ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			out := make([]string, len(questions))
			for i := range out {
				out[i] = secret
			}
			return out, nil
		}),
	}, nil
}

// connect opens the connection. With visible set it drives the first three stages; a reconnect after the host key
// is known passes without a question.
func (s *sshSession) connect(visible bool) error {
	methods, err := s.authMethods()
	if err != nil {
		if visible {
			s.setStep(2, "failed", err.Error(), "key_unreadable")
		}
		return err
	}
	addr := net.JoinHostPort(s.ip.String(), strconv.Itoa(s.port))
	if visible {
		s.setStep(0, "running", "", "")
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(s.ctx, "tcp", addr)
	if err != nil {
		if visible {
			s.setStep(0, "failed", err.Error(), "connect_failed")
		}
		return err
	}
	stop := context.AfterFunc(s.ctx, func() { _ = conn.Close() })
	defer stop()

	cfg := &ssh.ClientConfig{
		User: s.user, Auth: methods, Timeout: 15 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp := ssh.FingerprintSHA256(key)
			s.mu.Lock()
			known := s.hostFP
			s.mu.Unlock()
			if visible {
				s.setStep(0, "done", net.JoinHostPort(s.host, strconv.Itoa(s.port)), "")
				s.setStep(1, "running", "", "")
			}
			if known == fp {
				return nil
			}
			if known != "" {
				return fmt.Errorf("the host key changed since the check: %s", fp)
			}
			ans, err := s.askGate("hostkey", map[string]any{"fingerprint": fp, "type": key.Type()})
			if err != nil {
				return err
			}
			if !ans.Accept {
				return errHostKeyRejected
			}
			s.mu.Lock()
			s.hostFP = fp
			s.mu.Unlock()
			if visible {
				s.setStep(1, "done", key.Type()+" "+fp, "")
				s.setStep(2, "running", "", "")
			}
			return nil
		},
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		if visible {
			switch {
			case errors.Is(err, errHostKeyRejected):
				s.setStep(1, "failed", "", "hostkey_rejected")
			case strings.Contains(err.Error(), "unable to authenticate"):
				s.setStep(2, "failed", "", "auth_failed")
			default:
				s.setStep(0, "failed", err.Error(), "connect_failed")
			}
		}
		return err
	}
	client := ssh.NewClient(c, chans, reqs)
	s.mu.Lock()
	if s.client != nil {
		_ = s.client.Close()
	}
	s.client = client
	s.mu.Unlock()
	if visible {
		how := "password"
		if s.auth == "key" {
			how = "key"
		}
		s.setStep(2, "done", how, "")
	}
	return nil
}

// limitedBuffer keeps the first sshMaxOutput bytes. It is a plain type with a lock: the standard output and the
// standard error of a command are copied by two goroutines, and an embedded bytes.Buffer would let io.Copy bypass Write.
type limitedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.b.Len() < sshMaxOutput {
		_, _ = l.b.Write(p[:min(len(p), sshMaxOutput-l.b.Len())])
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// lineWriter splits what a command prints into log lines.
type lineWriter struct {
	buf  []byte
	emit func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 8192 {
		w.emit(string(w.buf))
		w.buf = nil
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}

func (s *sshSession) conn() (*ssh.Client, error) {
	s.mu.Lock()
	c := s.client
	s.mu.Unlock()
	if c == nil {
		return nil, errors.New("the SSH connection is closed")
	}
	return c, nil
}

// run executes a command and returns what it printed and its exit status.
func (s *sshSession) run(ctx context.Context, cmd, stdin string, limit time.Duration, out io.Writer) (string, int, error) {
	c, err := s.conn()
	if err != nil {
		return "", -1, err
	}
	sess, err := c.NewSession()
	if err != nil {
		return "", -1, err
	}
	defer func() { _ = sess.Close() }()
	buf := &limitedBuffer{}
	var w io.Writer = buf
	if out != nil {
		w = io.MultiWriter(buf, out)
	}
	sess.Stdout, sess.Stderr = w, w
	if stdin != "" {
		sess.Stdin = strings.NewReader(stdin)
	}
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case err = <-done:
	case <-timer.C:
		_ = sess.Close()
		return buf.String(), -1, errors.New("the command took too long")
	case <-ctx.Done():
		_ = sess.Close()
		return buf.String(), -1, ctx.Err()
	}
	if err == nil {
		return buf.String(), 0, nil
	}
	var ee *ssh.ExitError
	if errors.As(err, &ee) {
		return buf.String(), ee.ExitStatus(), nil
	}
	return buf.String(), -1, err
}

// priv runs a command with the rights of root: as root, through sudo without a password, or through sudo with the
// password that the administrator gave (it goes through the standard input, never into the command line).
func (s *sshSession) priv(ctx context.Context, cmd string, limit time.Duration, out io.Writer) (string, int, error) {
	s.mu.Lock()
	mode, pw := s.mode, s.sudoPW
	s.mu.Unlock()
	switch mode {
	case "root":
		return s.run(ctx, "sh -c "+shq(cmd), "", limit, out)
	case "nopass":
		return s.run(ctx, "sudo -n sh -c "+shq(cmd), "", limit, out)
	}
	return s.run(ctx, "sudo -S -p '' sh -c "+shq(cmd), pw+"\n", limit, out)
}

// ---- the checks ----

func parseKV(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			m[k] = v
		}
	}
	return m
}

var supportedOS = map[string]bool{"debian:12": true, "debian:13": true, "ubuntu:22.04": true, "ubuntu:24.04": true, "ubuntu:26.04": true}

const probeCmd = `. /etc/os-release 2>/dev/null
printf 'ID=%s\nVER=%s\nNAME=%s\n' "$ID" "$VERSION_ID" "$PRETTY_NAME"
printf 'ARCH=%s\n' "$(dpkg --print-architecture 2>/dev/null)"
if command -v wget >/dev/null 2>&1; then echo WGET=yes; else echo WGET=no; fi
if [ -d /run/systemd/system ]; then echo SYSTEMD=yes; else echo SYSTEMD=no; fi
echo FREE=$(df -Pk /usr/local/bin 2>/dev/null | awk 'NR==2{print int($4/1024)}')`

func (s *sshSession) failCheck(i int, code, detail string) {
	s.setStep(i, "failed", detail, code)
	s.mu.Lock()
	s.phase = "check_failed"
	s.errText = code
	s.mu.Unlock()
}

func (s *sshSession) runCheck() {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	ctx := s.ctx
	audit := func(result string) {
		if s.in.deps.Audit != nil {
			s.in.deps.Audit(context.WithoutCancel(ctx), s.login, "ssh.check", s.user+"@"+s.host, result, map[string]any{"port": s.port})
		}
	}
	if err := s.connect(true); err != nil {
		s.mu.Lock()
		if s.phase == "checking" {
			s.phase, s.errText = "check_failed", err.Error()
		}
		s.mu.Unlock()
		audit("error")
		return
	}
	// system and disk
	s.setStep(3, "running", "", "")
	out, _, err := s.run(ctx, probeCmd, "", 30*time.Second, nil)
	if err != nil {
		s.failCheck(3, "command_failed", err.Error())
		audit("error")
		return
	}
	kv := parseKV(out)
	name := kv["NAME"]
	if name == "" {
		name = kv["ID"] + " " + kv["VER"]
	}
	if !supportedOS[kv["ID"]+":"+kv["VER"]] || (kv["ARCH"] != "amd64" && kv["ARCH"] != "arm64") {
		det := strings.TrimSpace(name)
		if kv["ARCH"] != "" {
			det += " · " + kv["ARCH"]
		}
		s.failCheck(3, "os_unsupported", det)
		audit("error")
		return
	}
	s.mu.Lock()
	s.arch, s.hasWget = kv["ARCH"], kv["WGET"] == "yes"
	s.mu.Unlock()
	s.setStep(3, "done", name+" · "+kv["ARCH"], "")
	s.setStep(4, "running", "", "")
	free, _ := strconv.Atoi(kv["FREE"])
	switch {
	case kv["SYSTEMD"] != "yes":
		s.failCheck(4, "no_systemd", "")
		audit("error")
		return
	case free < 64:
		s.failCheck(4, "disk_low", fmt.Sprintf("%d MB", free))
		audit("error")
		return
	}
	s.setStep(4, "done", fmt.Sprintf("systemd · %d MB", free), "")

	// rights
	s.setStep(5, "running", "", "")
	if !s.checkRights(ctx) {
		audit("error")
		return
	}
	// reach: can the host connect to this server?
	s.setStep(6, "running", "", "")
	if !s.checkReach(ctx) {
		audit("error")
		return
	}
	// an agent that is there already
	s.setStep(7, "running", "", "")
	out, _, err = s.priv(ctx, `if [ -f /var/lib/linager-agent/state.json ]; then echo yes; ([ -x /usr/local/bin/linager ] && /usr/local/bin/linager version 2>/dev/null | head -1) || true; else echo no; fi`, 20*time.Second, nil)
	if err != nil {
		s.failCheck(7, "command_failed", err.Error())
		audit("error")
		return
	}
	if strings.HasPrefix(strings.TrimSpace(out), "yes") {
		det := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "yes"))
		s.setStep(7, "warn", det, "agent_exists")
		ans, err := s.askGate("reinstall", map[string]any{"version": det})
		if err != nil {
			return
		}
		if !ans.Accept {
			s.mu.Lock()
			s.phase, s.errText = "check_failed", "cancelled"
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		s.reinst = true
		s.mu.Unlock()
		s.setStep(7, "done", det, "")
	} else {
		s.setStep(7, "done", "", "")
	}
	s.mu.Lock()
	s.phase = "ready"
	s.mu.Unlock()
	audit("ok")
}

func (s *sshSession) checkRights(ctx context.Context) bool {
	out, _, err := s.run(ctx, "id -u", "", 15*time.Second, nil)
	if err != nil {
		s.failCheck(5, "command_failed", err.Error())
		return false
	}
	if strings.TrimSpace(out) == "0" {
		s.mu.Lock()
		s.mode = "root"
		s.mu.Unlock()
		s.setStep(5, "done", "root", "")
		return true
	}
	if _, code, err := s.run(ctx, "sudo -n true", "", 15*time.Second, nil); err == nil && code == 0 {
		s.mu.Lock()
		s.mode = "nopass"
		s.mu.Unlock()
		s.setStep(5, "done", "sudo", "")
		return true
	}
	if _, code, _ := s.run(ctx, "command -v sudo >/dev/null 2>&1", "", 15*time.Second, nil); code != 0 {
		s.failCheck(5, "sudo_missing", "")
		return false
	}
	retry := false
	for {
		s.setStep(5, "running", "", "sudo_password")
		ans, err := s.askGate("sudo", map[string]any{"retry": retry})
		if err != nil {
			return false
		}
		if !ans.Accept {
			s.failCheck(5, "sudo_denied", "")
			return false
		}
		_, code, err := s.run(ctx, "sudo -S -p '' true", ans.Password+"\n", 15*time.Second, nil)
		if err == nil && code == 0 {
			s.mu.Lock()
			s.mode, s.sudoPW = "password", ans.Password
			s.mu.Unlock()
			s.setStep(5, "done", "sudo", "")
			return true
		}
		retry = true
	}
}

var addrRe = regexp.MustCompile(`^[A-Za-z0-9.:\-\[\]]+$`)

func (s *sshSession) reachable(ctx context.Context, host string) bool {
	if !addrRe.MatchString(host) {
		return false
	}
	_, code, err := s.run(ctx, "timeout 5 bash -c "+shq("exec 3<>/dev/tcp/"+strings.Trim(host, "[]")+"/"+s.in.port()), "", 12*time.Second, nil)
	return err == nil && code == 0
}

func (s *sshSession) checkReach(ctx context.Context) bool {
	cands := s.in.candidates(s.extra)
	for {
		for _, h := range cands {
			if s.reachable(ctx, h) {
				s.mu.Lock()
				s.server = net.JoinHostPort(strings.Trim(h, "[]"), s.in.port())
				s.mu.Unlock()
				s.setStep(6, "done", s.server, "")
				return true
			}
		}
		s.setStep(6, "failed", strings.Join(cands, ", ")+" :"+s.in.port(), "port_unreachable")
		ans, err := s.askGate("address", map[string]any{"candidates": cands, "port": s.in.port()})
		if err != nil {
			return false
		}
		if !ans.Accept || strings.TrimSpace(ans.Address) == "" {
			s.mu.Lock()
			s.phase, s.errText = "check_failed", "port_unreachable"
			s.mu.Unlock()
			return false
		}
		cands = []string{strings.TrimSpace(ans.Address)}
		s.setStep(6, "running", "", "")
	}
}

// candidates are the addresses at which the new host may reach this server, the ones of the install links first.
func (in *SSHInstaller) candidates(extra string) []string {
	var out []string
	seen := map[string]bool{}
	for _, h := range append(in.deps.Hosts(), extra) {
		h = strings.Trim(h, "[]")
		if h != "" && !seen[h] && hostKind(h) != "" {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// ---- the install ----

// ErrBusy is returned when a run is going on.
var ErrBusy = errors.New("a run is going on")

// StartInstall begins the install after a successful check.
func (s *sshSession) StartInstall() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrBusy
	}
	if s.phase != "ready" && s.phase != "install_failed" {
		s.mu.Unlock()
		return errors.New("the checks have not passed")
	}
	s.phase, s.running, s.errText = "installing", true, ""
	s.steps = append(s.steps[:len(sshCheckIDs):len(sshCheckIDs)], newSSHSteps(sshInstallIDs)...)
	s.mu.Unlock()
	go s.runInstall()
	return nil
}

func (s *sshSession) fail(i int, code, detail string) {
	s.setStep(i, "failed", detail, code)
}

func (s *sshSession) runInstall() {
	base := len(sshCheckIDs)
	ctx := s.ctx
	deps := s.in.deps
	installed := false
	var serverID, tmp string
	finish := func(ok bool) {
		bg := context.WithoutCancel(ctx)
		if tmp != "" { // the files of the install go before the connection does
			_, _, _ = s.run(bg, "rm -rf "+shq(tmp), "", 20*time.Second, nil)
		}
		if s.tokenID != 0 && deps.EndToken != nil {
			deps.EndToken(bg, s.tokenID)
		}
		s.mu.Lock()
		s.running = false
		if ok {
			s.phase = "done"
			s.secret, s.phrase, s.sudoPW = "", "", ""
		} else {
			s.phase = "install_failed"
		}
		s.mu.Unlock()
		if ok {
			s.mu.Lock()
			if s.client != nil {
				_ = s.client.Close()
				s.client = nil
			}
			s.mu.Unlock()
		}
		result := "ok"
		if !ok {
			result = "error"
		}
		if deps.Audit != nil {
			deps.Audit(bg, s.login, "ssh.install", s.user+"@"+s.host, result, map[string]any{"port": s.port, "server": s.newName})
		}
	}
	rollback := func() {
		bg := context.WithoutCancel(ctx)
		if installed {
			idx := s.addStep("rollback")
			s.setStep(idx, "running", "", "")
			_, _, _ = s.priv(bg, `if [ -x /usr/local/bin/linager ]; then /usr/local/bin/linager agent uninstall --purge; fi`, time.Minute, nil)
			s.setStep(idx, "done", "", "")
			s.logf("agent removed from the host")
		}
		if serverID != "" && deps.Forget != nil {
			deps.Forget(bg, serverID)
		}
	}

	// the client may have gone while the administrator read the screen
	if _, err := s.conn(); err != nil {
		if err := s.connect(false); err != nil {
			s.fail(base, "connect_failed", err.Error())
			finish(false)
			return
		}
	}

	// 1. token
	s.setStep(base, "running", "", "")
	secret, tid, err := deps.NewToken(ctx, s.login, s.groupID)
	if err != nil {
		s.fail(base, "token_failed", err.Error())
		finish(false)
		return
	}
	s.mu.Lock()
	s.tokenID, s.mask = tid, secret
	arch, addr, reinst, hasWget := s.arch, s.server, s.reinst, s.hasWget
	s.mu.Unlock()
	s.setStep(base, "done", "", "")
	assets, err := deps.Assets(ctx)
	asset, ok := assets[arch]
	if err != nil || !ok {
		s.fail(base+1, "release_missing", arch)
		finish(false)
		return
	}

	dir, code, err := s.run(ctx, "mktemp -d /tmp/linager-install.XXXXXX", "", 20*time.Second, nil)
	if dir = strings.TrimSpace(dir); err != nil || code != 0 || dir == "" {
		s.fail(base+1, "command_failed", "mktemp")
		finish(false)
		return
	}
	tmp = dir

	// 2. download
	s.setStep(base+1, "running", "", "")
	s.setProgress(base+1, 0, asset.Size)
	if !hasWget {
		s.logf("wget is missing on the host: installing it")
		if _, c, err := s.priv(ctx, "DEBIAN_FRONTEND=noninteractive apt-get install -y wget ca-certificates", 5*time.Minute, nil); err != nil || c != 0 {
			s.fail(base+1, "wget_failed", "")
			finish(false)
			return
		}
	}
	url := "https://" + addr + "/v1/join/" + secret + "/binary?arch=" + arch
	dl := "wget -nv --no-check-certificate --pinnedpubkey " + shq(deps.Pin) + " -O " + shq(tmp+"/linager") + " " + shq(url)
	s.logf("$ %s", strings.ReplaceAll(dl, shq(url), shq("https://"+addr+"/v1/join/••••/binary?arch="+arch)))
	pctx, stopProgress := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-t.C:
				out, _, err := s.run(pctx, "stat -c %s "+shq(tmp+"/linager"), "", 5*time.Second, nil)
				if n, perr := strconv.ParseInt(strings.TrimSpace(out), 10, 64); err == nil && perr == nil {
					s.setProgress(base+1, n, asset.Size)
				}
			}
		}
	}()
	out, code, err := s.run(ctx, dl, "", 10*time.Minute, nil)
	stopProgress()
	if err != nil || code != 0 {
		s.logf("%s", strings.TrimSpace(out))
		s.fail(base+1, "download_failed", strings.TrimSpace(out))
		finish(false)
		return
	}
	s.setProgress(base+1, asset.Size, asset.Size)
	s.setStep(base+1, "done", "", "")
	s.logf("downloaded %d bytes", asset.Size)

	// 3. verify against the signed manifest
	s.setStep(base+2, "running", "", "")
	out, code, err = s.run(ctx, "sha256sum "+shq(tmp+"/linager"), "", 60*time.Second, nil)
	if err != nil || code != 0 || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(out)), strings.ToLower(asset.SHA256)) {
		s.fail(base+2, "checksum_mismatch", "")
		finish(false)
		return
	}
	s.setStep(base+2, "done", asset.SHA256[:12], "")
	s.logf("the checksum matches the signed release")

	// 4. install: the same installer as the link runs
	s.setStep(base+3, "running", "", "")
	force := ""
	if reinst {
		force = " --force"
	}
	inst := fmt.Sprintf("chmod 0755 %s && %s install agent --no-tui --server %s --fingerprint %s --join-token %s%s",
		shq(tmp+"/linager"), shq(tmp+"/linager"), shq(addr), shq(deps.Fingerprint), shq(secret), force)
	s.logf("$ linager install agent --no-tui --server %s --fingerprint %s --join-token ••••%s", addr, deps.Fingerprint, force)
	installed = true
	lw := &lineWriter{emit: func(l string) { s.logf("%s", l) }}
	_, code, err = s.priv(ctx, inst, 5*time.Minute, lw)
	lw.flush()
	if err != nil || code != 0 {
		det := "exit " + strconv.Itoa(code)
		if err != nil {
			det = err.Error()
		}
		s.fail(base+3, "install_failed", det)
		rollback()
		finish(false)
		return
	}
	s.setStep(base+3, "done", "", "")

	// 5. the server has taken the request (the token approves it at once)
	s.setStep(base+4, "running", "", "")
	var name string
	var online bool
	for deadline := time.Now().Add(sshBindMax); ; {
		var bound bool
		serverID, name, bound, online = deps.Observe(ctx, tid)
		if bound {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			s.fail(base+4, "not_bound", "")
			rollback()
			finish(false)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	s.setStep(base+4, "done", name, "")
	s.logf("the server accepted the agent as %s", name)

	// 6. the first connection of the agent
	s.setStep(base+5, "running", "", "")
	for deadline := time.Now().Add(sshConnectMax); !online; {
		if time.Now().After(deadline) || ctx.Err() != nil {
			s.fail(base+5, "not_connected", "")
			s.logf("the agent did not connect within %d s", int(sshConnectMax/time.Second))
			rollback()
			finish(false)
			return
		}
		time.Sleep(time.Second)
		_, name, _, online = deps.Observe(ctx, tid)
	}
	s.mu.Lock()
	s.newID, s.newName = serverID, name
	s.mu.Unlock()
	s.setStep(base+5, "done", name, "")
	s.logf("the agent is connected, the name of the server is %s", name)
	finish(true)
}

// addStep appends a stage (the rollback) and returns its index.
func (s *sshSession) addStep(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, sshStep{ID: id, State: "pending"})
	return len(s.steps) - 1
}

// port is the agent port the new host connects to.
func (in *SSHInstaller) port() string {
	in.portMu.RLock()
	defer in.portMu.RUnlock()
	return in.deps.Port
}

// SetPort follows a change of the agent port.
func (in *SSHInstaller) SetPort(p string) {
	in.portMu.Lock()
	in.deps.Port = p
	in.portMu.Unlock()
}
