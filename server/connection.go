package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/internal/protocol"
	"github.com/bolgov0zero/linager/internal/release"
	"github.com/bolgov0zero/linager/server/store"
)

// settingConnection is the settings key that holds the ports changed in the panel. Once it exists it wins over
// server.yaml (the service cannot write the file, the database is where the panel keeps what it changes).
const settingConnection = "connection"

// ConnState is the stored part of the connection settings.
type ConnState struct {
	AgentPort int `json:"agent_port"`
	// AppPort is the port of the desktop application listener; 0 switches it off.
	AppPort int `json:"app_port"`
	// LegacyPort is the previous agent port. It stays open while agents move to the new one; 0 means no move.
	LegacyPort int `json:"legacy_port"`
}

// ConnError is a refused change with a code the panel can translate.
type ConnError struct {
	Status   int
	Code     string
	Detail   string
	Blockers []ConnAgent
}

func (e *ConnError) Error() string { return e.Code + ": " + e.Detail }

// ConnAgent is an agent in a list of the connection settings.
type ConnAgent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Online    bool   `json:"online"`
	Supported bool   `json:"supported"` // the agent follows a change of the agent port
}

// ConnMove is the progress of a move of the agent port.
type ConnMove struct {
	From    int         `json:"from"`
	To      int         `json:"to"`
	Moved   int         `json:"moved"`
	Total   int         `json:"total"`
	Pending []ConnAgent `json:"pending"`
}

// ConnStatus is what the panel shows.
type ConnStatus struct {
	AgentPort   int         `json:"agent_port"`
	AppPort     int         `json:"app_port"`
	Host        string      `json:"host"`        // the address the listeners are bound to
	Fingerprint string      `json:"fingerprint"` // of the certificate of both ports
	MinAgent    string      `json:"min_agent"`   // the first agent version that follows a change of the agent port
	AdminPort   int         `json:"admin_port"`
	AppUp       bool        `json:"app_up"` // the application listener is really open
	Move        *ConnMove   `json:"move"`
	Blockers    []ConnAgent `json:"blockers"` // online agents that would be cut off by a change of the agent port
}

// ConnReq is what the panel sends.
type ConnReq struct {
	AgentPort int `json:"agent_port"`
	AppPort   int `json:"app_port"`
}

type portListener struct {
	ln     net.Listener
	port   int
	closed atomic.Bool
}

func (p *portListener) close() {
	p.closed.Store(true)
	_ = p.ln.Close()
}

// Connection owns the listeners of the agent port and of the application port and changes them while the server runs.
type Connection struct {
	db      *store.Store
	hub     *Hub
	agents  *AgentPort
	ssh     *SSHInstaller // follows the agent port (nil in tests)
	keys    *Keys
	log     *slog.Logger
	adminLn string // the admin listen address, for the conflict check

	agentHost, appHost string

	agentSrv, appSrv *http.Server
	errc             chan<- error

	mu     sync.Mutex
	st     ConnState
	agent  *portListener
	legacy *portListener
	app    *portListener
}

// NewConnection reads the ports from the configuration and from the settings stored by the panel.
func NewConnection(ctx context.Context, cfg *config.Server, db *store.Store, hub *Hub, agents *AgentPort, keys *Keys, log *slog.Logger) (*Connection, error) {
	ah, ap, err := net.SplitHostPort(cfg.Agents.Listen)
	if err != nil {
		return nil, err
	}
	c := &Connection{db: db, hub: hub, agents: agents, keys: keys, log: log, adminLn: cfg.Admin.Listen, agentHost: ah, appHost: ah}
	c.st.AgentPort, _ = strconv.Atoi(ap)
	if cfg.App.Listen != "" {
		if h, p, err := net.SplitHostPort(cfg.App.Listen); err == nil {
			c.appHost = h
			c.st.AppPort, _ = strconv.Atoi(p)
		}
	}
	if raw := db.SettingRaw(ctx, settingConnection); raw != nil {
		var st ConnState
		if json.Unmarshal(raw, &st) == nil && st.AgentPort > 0 {
			c.st = st
		}
	}
	agents.SetListenPort(strconv.Itoa(c.st.AgentPort))
	return c, nil
}

// State returns the current ports.
func (c *Connection) State() ConnState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st
}

func (c *Connection) agentServer() *http.Server {
	return &http.Server{Handler: c.agents.Handler(), TLSConfig: c.agents.TLSConfig(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
}

// appTLS is the certificate of the agent port, TLS 1.3 only: the application is our own client.
func (c *Connection) appTLS() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{c.keys.TLS, c.keys.TLSAlt}}
}

// stripForwarded removes what a reverse proxy would add. The application port is a direct connection: a client must
// not be able to name its own address (the access restriction and the sign-in limits judge it).
func stripForwarded(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Port", "X-Real-Ip", "Forwarded"} {
			r.Header.Del(h)
		}
		next.ServeHTTP(w, r)
	})
}

// Start opens the listeners: the agent port (an error is fatal), the old agent port of a move in progress and the
// application port (an error is only logged: the server works without it).
func (c *Connection) Start(errc chan<- error, appHandler http.Handler) error {
	c.errc = errc
	c.agentSrv = c.agentServer()
	c.appSrv = &http.Server{Handler: stripForwarded(appHandler), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if c.agent, err = c.open(c.agentSrv, c.agentHost, c.st.AgentPort, c.agents.TLSConfig(), true); err != nil {
		return err
	}
	if c.st.LegacyPort != 0 {
		if c.legacy, err = c.open(c.agentSrv, c.agentHost, c.st.LegacyPort, c.agents.TLSConfig(), false); err != nil {
			c.log.Error("the old agent port of a move in progress cannot be opened", "port", c.st.LegacyPort, "err", err)
		}
	}
	if c.st.AppPort != 0 {
		if c.app, err = c.open(c.appSrv, c.appHost, c.st.AppPort, c.appTLS(), false); err != nil {
			c.log.Error("the application port cannot be opened", "port", c.st.AppPort, "err", err)
		}
	}
	return nil
}

// open binds a port and serves it. fatal tells whether a failure of the accept loop ends the server.
func (c *Connection) open(srv *http.Server, host string, port int, cfg *tls.Config, fatal bool) (*portListener, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	pl := &portListener{ln: ln, port: port}
	go func() {
		err := srv.Serve(tls.NewListener(ln, cfg))
		if pl.closed.Load() || errors.Is(err, http.ErrServerClosed) {
			return
		}
		if fatal {
			c.errc <- err
		} else {
			c.log.Error("listener stopped", "port", port, "err", err)
		}
	}()
	return pl, nil
}

// Shutdown stops both servers.
func (c *Connection) Shutdown(ctx context.Context) {
	if c.agentSrv != nil {
		_ = c.agentSrv.Shutdown(ctx)
	}
	if c.appSrv != nil {
		_ = c.appSrv.Shutdown(ctx)
	}
}

func (c *Connection) save(ctx context.Context, st ConnState) error {
	b, _ := json.Marshal(st)
	return c.db.SetSettingRaw(ctx, settingConnection, b)
}

// supportsMove reports whether an agent of this version follows a change of the agent port. A build without a
// version number ("dev") is taken for a current one; an agent that never reported a version is not.
func supportsMove(version string) bool {
	v := release.ParseVersion(version)
	if !v.Valid {
		return version != ""
	}
	return release.Compare(v, release.ParseVersion(protocol.MoveMinAgent)) >= 0
}

func (c *Connection) agentList(ctx context.Context) ([]ConnAgent, []store.ServerPort, error) {
	rows, err := c.db.ServerPorts(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]ConnAgent, len(rows))
	for i, r := range rows {
		out[i] = ConnAgent{ID: r.ID, Name: r.Name, Version: r.Version, Online: c.hub.Online(r.ID), Supported: supportsMove(r.Version)}
	}
	return out, rows, nil
}

// Status describes the settings and the move in progress.
func (c *Connection) Status(ctx context.Context) (*ConnStatus, error) {
	st := c.State()
	list, rows, err := c.agentList(ctx)
	if err != nil {
		return nil, err
	}
	_, adminPort, _ := net.SplitHostPort(c.adminLn)
	ap, _ := strconv.Atoi(adminPort)
	c.mu.Lock()
	appUp := c.app != nil
	c.mu.Unlock()
	out := &ConnStatus{AgentPort: st.AgentPort, AppPort: st.AppPort, Host: c.agentHost, Fingerprint: pki.Fingerprint(c.keys.TLSDER), MinAgent: protocol.MoveMinAgent,
		AdminPort: ap, AppUp: appUp, Blockers: []ConnAgent{}}
	for _, a := range list {
		if a.Online && !a.Supported {
			out.Blockers = append(out.Blockers, a)
		}
	}
	if st.LegacyPort != 0 {
		mv := &ConnMove{From: st.LegacyPort, To: st.AgentPort, Total: len(rows), Pending: []ConnAgent{}}
		for i, r := range rows {
			if r.LastPort == st.AgentPort {
				mv.Moved++
			} else {
				mv.Pending = append(mv.Pending, list[i])
			}
		}
		out.Move = mv
	}
	return out, nil
}

func portOf(addr string) int {
	_, p, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(p)
	return n
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

// Apply changes the ports. New ports are opened before anything is closed or stored, so a busy port leaves
// everything as it was.
func (c *Connection) Apply(ctx context.Context, req ConnReq) error {
	if !validPort(req.AgentPort) || (req.AppPort != 0 && !validPort(req.AppPort)) {
		return &ConnError{Status: http.StatusBadRequest, Code: "bad_port", Detail: "a port is 1-65535"}
	}
	adminPort := portOf(c.adminLn)
	switch {
	case req.AgentPort == adminPort || (req.AppPort != 0 && req.AppPort == adminPort):
		return &ConnError{Status: http.StatusConflict, Code: "port_conflict", Detail: "the port is used by the admin panel"}
	case req.AppPort != 0 && req.AppPort == req.AgentPort:
		return &ConnError{Status: http.StatusConflict, Code: "port_conflict", Detail: "the agent port and the application port must differ"}
	}
	cur := c.State()
	agentChanged, appChanged := req.AgentPort != cur.AgentPort, req.AppPort != cur.AppPort
	if !agentChanged && !appChanged {
		return nil
	}
	if agentChanged {
		if cur.LegacyPort != 0 {
			return &ConnError{Status: http.StatusConflict, Code: "move_active", Detail: "agents are still moving to the current port; finish the move or close the old port first"}
		}
		if req.AgentPort == cur.AppPort || req.AgentPort == cur.LegacyPort {
			return &ConnError{Status: http.StatusConflict, Code: "port_conflict", Detail: "the port is used by the application listener"}
		}
		list, _, err := c.agentList(ctx)
		if err != nil {
			return err
		}
		var blockers []ConnAgent
		for _, a := range list {
			if a.Online && !a.Supported {
				blockers = append(blockers, a)
			}
		}
		if len(blockers) > 0 {
			return &ConnError{Status: http.StatusConflict, Code: "agents_outdated", Detail: "update these agents first", Blockers: blockers}
		}
	}
	if appChanged && req.AppPort != 0 && (req.AppPort == cur.AgentPort || req.AppPort == cur.LegacyPort) {
		return &ConnError{Status: http.StatusConflict, Code: "port_conflict", Detail: "the port is used by the agent listener"}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	var newAgent, newApp *portListener
	var err error
	if agentChanged {
		if newAgent, err = c.open(c.agentSrv, c.agentHost, req.AgentPort, c.agents.TLSConfig(), false); err != nil {
			return &ConnError{Status: http.StatusConflict, Code: "port_in_use", Detail: err.Error()}
		}
	}
	if appChanged && req.AppPort != 0 {
		if newApp, err = c.open(c.appSrv, c.appHost, req.AppPort, c.appTLS(), false); err != nil {
			if newAgent != nil {
				newAgent.close()
			}
			return &ConnError{Status: http.StatusConflict, Code: "port_in_use", Detail: err.Error()}
		}
	}
	next := c.st
	next.AgentPort, next.AppPort = req.AgentPort, req.AppPort
	if agentChanged {
		next.LegacyPort = cur.AgentPort
	}
	if err := c.save(ctx, next); err != nil {
		if newAgent != nil {
			newAgent.close()
		}
		if newApp != nil {
			newApp.close()
		}
		return err
	}
	c.st = next
	if appChanged {
		if c.app != nil {
			c.app.close()
		}
		c.app = newApp
	}
	if agentChanged {
		c.legacy, c.agent = c.agent, newAgent
		c.agents.SetListenPort(strconv.Itoa(req.AgentPort))
		if c.ssh != nil {
			c.ssh.SetPort(strconv.Itoa(req.AgentPort))
		}
		go c.announce(context.WithoutCancel(ctx), req.AgentPort)
	}
	return nil
}

// announce tells every connected agent about the new port, then closes the old one when nobody is left to move.
func (c *Connection) announce(ctx context.Context, port int) {
	list, _, err := c.agentList(ctx)
	if err == nil {
		for _, a := range list {
			if a.Online && a.Supported {
				if err := c.hub.Send(ctx, a.ID, protocol.TypeServerMove, protocol.ServerMove{Port: port}); err != nil {
					c.log.Warn("move notice not sent", "server", a.Name, "err", err)
				}
			}
		}
	}
	c.maybeFinish(ctx)
}

// AgentConnected is called when an agent connected on the local port. It records the port and, while a move is in
// progress, sends the agent that is still on the old port the new one (an agent that was offline meanwhile).
func (c *Connection) AgentConnected(ctx context.Context, id, version string, local int) {
	if local > 0 {
		_ = c.db.SetServerPort(ctx, id, local)
	}
	st := c.State()
	if st.LegacyPort == 0 {
		return
	}
	if local == st.AgentPort {
		go c.maybeFinish(context.WithoutCancel(ctx))
		return
	}
	if supportsMove(version) {
		_ = c.hub.Send(ctx, id, protocol.TypeServerMove, protocol.ServerMove{Port: st.AgentPort})
	}
}

// maybeFinish closes the old port once every registered agent is on the new one.
func (c *Connection) maybeFinish(ctx context.Context) {
	if c.State().LegacyPort == 0 {
		return
	}
	rows, err := c.db.ServerPorts(ctx)
	if err != nil {
		return
	}
	cur := c.State().AgentPort
	for _, r := range rows {
		if r.LastPort != cur {
			return
		}
	}
	if err := c.CloseLegacy(ctx); err == nil {
		c.log.Info("every agent moved, the old agent port is closed")
	}
}

// CloseLegacy closes the old agent port. Agents that did not move lose their connection.
func (c *Connection) CloseLegacy(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.LegacyPort == 0 {
		return &ConnError{Status: http.StatusConflict, Code: "no_move", Detail: "there is no old port to close"}
	}
	next := c.st
	next.LegacyPort = 0
	if err := c.save(ctx, next); err != nil {
		return err
	}
	c.st = next
	if c.legacy != nil {
		c.legacy.close()
		c.legacy = nil
	}
	return nil
}

// Describe is a one-line summary for the log and the audit journal.
func (s ConnState) Describe() string {
	return fmt.Sprintf("agents %d, application %d", s.AgentPort, s.AppPort)
}
