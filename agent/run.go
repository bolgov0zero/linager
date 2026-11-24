package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/protocol"
)

// ErrRevoked means the server no longer knows this agent (it was deleted in the application).
var ErrRevoked = errors.New("this agent was removed from the server")

// ErrUninstalled is returned after the server asked the agent to remove itself.
var ErrUninstalled = errors.New("agent was uninstalled by the server")

// ErrCertExpired means the server refused the agent certificate because it is no longer valid.
var ErrCertExpired = errors.New("the agent certificate has expired")

// RunOptions configure the connection loop.
type RunOptions struct {
	DataDir      string
	AgentVersion string
	Log          *slog.Logger
	PingEvery    time.Duration // default 15 s
	SampleEvery  time.Duration // default 5 s
	// Collector supplies samples; nil uses the /proc collector.
	Collector Collector
	// Config selects the enabled modules; nil enables all of them.
	Config *config.Agent
	// Uninstall performs self-removal; nil uses the real implementation.
	Uninstall func(purge bool, dataDir string) error
	// RenewRetryEvery is the longest pause between certificate renewal requests (default 12 h).
	RenewRetryEvery time.Duration
	// ExtraStreams adds stream handlers to the registry (the tests and the simulator use it to stand in for a
	// module the host does not have).
	ExtraStreams map[string]StreamFunc
}

// Run keeps a signed WebSocket connection to the server until ctx ends,
// the agent is revoked, or it is told to uninstall itself.
func Run(ctx context.Context, o RunOptions) error {
	if o.PingEvery == 0 {
		o.PingEvery = 15 * time.Second
	}
	if o.SampleEvery == 0 {
		o.SampleEvery = 5 * time.Second
	}
	if o.RenewRetryEvery == 0 {
		o.RenewRetryEvery = 12 * time.Hour
	}
	if o.Uninstall == nil {
		o.Uninstall = Uninstall
	}
	if o.Collector == nil {
		o.Collector = NewCollector(ctx)
	}
	if o.Config == nil {
		o.Config = config.DefaultAgent()
	}
	reg := NewRegistry(o.Config, o.Collector, o.Log, o.DataDir)
	for name, fn := range o.ExtraStreams {
		reg.streams[name] = fn
	}
	reg.startBackground(ctx)
	buf := &sampleBuffer{}
	go sampler(ctx, o, buf)
	st, cert, err := loadState(o.DataDir)
	if err != nil {
		return err
	}
	key, err := loadOrCreateKey(o.DataDir)
	if err != nil {
		return err
	}
	backoff := minBackoff
	expiredLogged := false
	for {
		// The certificate is re-read before every connection: a renewed one is picked up without a restart.
		if s2, c2, lerr := loadState(o.DataDir); lerr == nil {
			st, cert = s2, c2
		}
		started := time.Now()
		tryingNext := st.Next != ""
		err := session(ctx, o, st, cert, key, buf, reg)
		if tryingNext && st.Next != "" && ctx.Err() == nil && !errors.Is(err, errMoved) { // the new address did not work
			if back, serr := failMove(o.DataDir, st, time.Now()); serr != nil {
				o.Log.Error("move: state not saved", "err", serr)
			} else if back {
				o.Log.Warn("the new server port does not work, staying on the old one", "err", err)
			}
		}
		switch {
		case errors.Is(err, ErrRevoked), errors.Is(err, ErrUninstalled), ctx.Err() != nil:
			if ctx.Err() != nil && err == nil {
				return ctx.Err()
			}
			return err
		}
		wait := reconnectPause(backoff, time.Since(started))
		if errors.Is(err, ErrCertExpired) {
			if !expiredLogged {
				o.Log.Error("the agent certificate has expired and the server refuses it; enroll this host again with `linager install`")
				expiredLogged = true
			}
			wait = maxBackoff
		}
		o.Log.Warn("disconnected", "err", err, "retry_in", wait.String())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait + time.Duration(rand.Int64N(int64(wait/2)+1))): //nolint:gosec // jitter, not security
		}
		backoff = min(wait*2, maxBackoff)
	}
}

const (
	minBackoff = time.Second
	maxBackoff = time.Minute
	// A session that lasted this long was a working connection: the next outage starts from a short pause again.
	healthySession = 30 * time.Second
)

// reconnectPause returns the pause before the next connection attempt. The pause grows while attempts
// keep failing (the caller doubles it) and starts over after a session that stayed connected for a while.
func reconnectPause(backoff, sessionLasted time.Duration) time.Duration {
	if sessionLasted >= healthySession || backoff < minBackoff {
		return minBackoff
	}
	return min(backoff, maxBackoff)
}

func session(ctx context.Context, o RunOptions, st *State, cert tls.Certificate, key ed25519.PrivateKey, buf *sampleBuffer, reg *Registry) error {
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: pinnedTLS(st.ServerFP, &cert)}}
	leaf, _ := leafOf(cert)
	target := st.Server
	if st.Next != "" { // the server asked the agent to move: the new address is tried first
		target = st.Next
	}
	ws, resp, err := websocket.Dial(ctx, "wss://"+target+"/v1/ws", &websocket.DialOptions{HTTPClient: hc})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			switch resp.Header.Get("X-Linager-Reason") {
			case protocol.ReasonRevoked:
				return ErrRevoked
			case protocol.ReasonExpired:
				return ErrCertExpired
			}
		}
		// An expired client certificate is normally refused during the TLS handshake already.
		if leaf != nil && time.Now().After(leaf.NotAfter) {
			return errors.Join(ErrCertExpired, err)
		}
		return err
	}
	ws.SetReadLimit(1 << 20)
	defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer reg.CloseAll()

	var seq uint64
	var sendMu sync.Mutex // seq numbers must hit the wire in order
	send := func(typ string, payload any) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		seq++
		env, err := protocol.Sign(key, typ, seq, payload)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(env)
		return ws.Write(ctx, websocket.MessageText, b)
	}
	if err := send(protocol.TypeHello, protocol.Hello{AgentVersion: o.AgentVersion, OS: helloOS(), Hostname: hostname(), Modules: reg.Modules()}); err != nil {
		return err
	}

	var guard protocol.SeqGuard
	read := func() (*protocol.Envelope, error) {
		_, b, err := ws.Read(ctx)
		if err != nil {
			return nil, err
		}
		var env protocol.Envelope
		if err := json.Unmarshal(b, &env); err != nil {
			return nil, err
		}
		if err := protocol.Verify(ed25519.PublicKey(st.CmdPubkey), &env, time.Now()); err != nil {
			return nil, err
		}
		return &env, guard.Accept(env.Seq)
	}
	hello, err := read()
	if err != nil || hello.Type != protocol.TypeHello {
		return errors.New("bad hello from server")
	}
	reg.SetEmitter(send)
	defer reg.SetEmitter(nil)
	if ConfirmUpdate(o.DataDir, func(m string, a ...any) { o.Log.Info(m, a...) }) { // the new version is connected: cancel the rollback
		reg.emitUpdate(protocol.UpdateEvent{Version: o.AgentVersion, Step: 6, Total: 6, ID: "confirm", State: protocol.UpdateDone})
	}
	if st.Next != "" { // the new address answers and the server knows this agent: it becomes the main one
		if err := promoteMove(o.DataDir, st); err != nil {
			o.Log.Error("move: state not saved", "err", err)
		} else {
			o.Log.Info("moved to the new server port", "server", st.Server)
		}
	}
	o.Log.Info("connected", "server", st.Server)

	// Certificate renewal: asked over the signed channel when a third of the lifetime is left, and again
	// after every renewal, so a connection that stays up for years never runs out of certificate.
	renewedCh := make(chan *x509.Certificate, 1)
	go func(cur *x509.Certificate) {
		for cur != nil {
			wait := time.Until(renewDue(cur))
			if wait <= 0 {
				if send(protocol.TypeCertRenew, nil) != nil {
					return
				}
				wait = renewRetry(cur, o.RenewRetryEvery)
			}
			select {
			case <-ctx.Done():
				return
			case cur = <-renewedCh:
			case <-time.After(wait):
			}
		}
	}(leaf)

	errc := make(chan error, 1)
	go func() {
		t := time.NewTicker(o.PingEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := send(protocol.TypePing, nil); err != nil {
					errc <- err
					return
				}
			}
		}
	}()
	go func() {
		t := time.NewTicker(o.SampleEvery)
		defer t.Stop()
		first := true
		host := hostname()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// The hostname is polled every tick (a cheap uname); a change resends the system info so the
				// server can update the server name without waiting for a reconnect.
				renamed := false
				if h := hostname(); h != host {
					host, renamed = h, true
				}
				pending := buf.snapshot()
				if len(pending) == 0 && !first && !renamed {
					continue
				}
				b := protocol.MetricsBatch{Samples: pending}
				if first || renamed {
					si := o.Collector.System(o.AgentVersion)
					b.System = &si
				}
				if err := send(protocol.TypeMetrics, b); err != nil {
					errc <- err
					return
				}
				buf.ack(len(pending))
				first = false
			}
		}
	}()
	msgs := make(chan *protocol.Envelope)
	go func() {
		for {
			env, err := read()
			if err != nil {
				errc <- err
				return
			}
			select {
			case msgs <- env:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errc:
			return err
		case env := <-msgs:
			switch env.Type {
			case protocol.TypeCmd:
				var c protocol.Cmd
				if json.Unmarshal(env.Payload, &c) == nil {
					go func() { _ = send(protocol.TypeCmdResult, reg.HandleCmd(ctx, c)) }()
				}
				continue
			case protocol.TypeStreamOpen:
				var so protocol.StreamOpen
				if json.Unmarshal(env.Payload, &so) == nil {
					reg.OpenStream(ctx, so, send)
				}
				continue
			case protocol.TypeStreamInput:
				var d protocol.StreamData
				if json.Unmarshal(env.Payload, &d) == nil {
					reg.StreamInput(d.SID, d.Data)
				}
				continue
			case protocol.TypeStreamResize:
				var rz protocol.StreamResize
				if json.Unmarshal(env.Payload, &rz) == nil {
					reg.StreamResize(rz.SID, rz.Cols, rz.Rows)
				}
				continue
			case protocol.TypeStreamClose:
				var sc protocol.StreamClose
				if json.Unmarshal(env.Payload, &sc) == nil {
					reg.StreamClose(sc.SID)
				}
				continue
			}
			if env.Type == protocol.TypeCertRenewed {
				var cr protocol.CertRenewed
				if json.Unmarshal(env.Payload, &cr) != nil || leaf == nil {
					continue
				}
				nl, err := installRenewedCert(o.DataDir, key, leaf, []byte(cr.CertPEM), time.Now())
				if err != nil {
					o.Log.Error("renewed certificate rejected", "err", err)
					continue
				}
				leaf = nl // the new certificate is used from the next connection on
				select {
				case renewedCh <- nl:
				default:
				}
				o.Log.Info("agent certificate renewed", "valid_until", nl.NotAfter.Format(time.RFC3339))
				continue
			}
			if env.Type == protocol.TypeServerMove {
				var m protocol.ServerMove
				if json.Unmarshal(env.Payload, &m) != nil {
					continue
				}
				switch started, err := startMove(o.DataDir, st, m.Port, time.Now()); {
				case err != nil:
					o.Log.Error("move: state not saved", "err", err)
				case started:
					o.Log.Info("the server changed its agent port, reconnecting", "port", m.Port)
					return errMoved
				}
				continue
			}
			if env.Type == protocol.TypeAgentRemove {
				var u protocol.Uninstall
				_ = json.Unmarshal(env.Payload, &u)
				o.Log.Warn("server requested agent removal", "purge", u.Purge)
				if err := o.Uninstall(u.Purge, o.DataDir); err != nil {
					o.Log.Error("uninstall failed", "err", err)
					continue
				}
				return ErrUninstalled
			}
		}
	}
}

// maxBuffered is how many samples are kept while the server is unreachable (one hour at 5 s).
const maxBuffered = 720

// sampleBuffer holds samples not yet delivered to the server.
type sampleBuffer struct {
	mu    sync.Mutex
	items []protocol.Sample
}

func (b *sampleBuffer) add(s protocol.Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(b.items, s)
	if len(b.items) > maxBuffered {
		b.items = b.items[len(b.items)-maxBuffered:]
	}
}

func (b *sampleBuffer) snapshot() []protocol.Sample {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]protocol.Sample(nil), b.items...)
}

// ack removes the first n samples after they were written to the connection.
func (b *sampleBuffer) ack(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > len(b.items) {
		n = len(b.items)
	}
	b.items = b.items[n:]
}

// sampler collects a sample every SampleEvery, independent of the connection state.
func sampler(ctx context.Context, o RunOptions, buf *sampleBuffer) {
	o.Collector.Collect(time.Now()) // primes the CPU and network counters
	t := time.NewTicker(o.SampleEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			buf.add(o.Collector.Collect(now))
		}
	}
}
