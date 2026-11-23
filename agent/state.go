// Package agent implements the Linager agent: enrollment, the signed server connection
// and self-removal.
package agent

import (
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/bolgov0zero/linager/internal/pki"
)

// State is what the agent keeps after a successful enrollment.
type State struct {
	Server      string `json:"server"` // host:port of the agent port
	ServerFP    string `json:"server_fp"`
	ServerID    string `json:"server_id"`
	CmdPubkey   []byte `json:"cmd_pubkey"`
	Fingerprint string `json:"-"`
	// Next is the address the server asked the agent to move to (host:port). It is tried first; the first
	// connection that works makes it Server, and a few failures drop it again (BadPort remembers the port so the
	// server's next request for it is ignored for a while).
	Next      string `json:"next,omitempty"`
	NextTries int    `json:"next_tries,omitempty"`
	BadPort   int    `json:"bad_port,omitempty"`
	BadUntil  int64  `json:"bad_until,omitempty"` // unix seconds
}

// Files inside the data directory.
const (
	keyFile   = "agent.key"
	certFile  = "agent.crt"
	stateFile = "state.json"
)

// ErrNotEnrolled means the agent has no state yet.
var ErrNotEnrolled = errors.New("agent is not enrolled; run `linager agent enroll` first")

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return secureDir(dir)
}

func loadOrCreateKey(dir string) (ed25519.PrivateKey, error) {
	p := filepath.Join(dir, keyFile)
	if b, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed name inside the data dir
		return pki.ParsePrivateKey(b)
	}
	if err := ensureDir(dir); err != nil {
		return nil, err
	}
	_, priv, err := pki.GenerateKey()
	if err != nil {
		return nil, err
	}
	return priv, os.WriteFile(p, pki.MarshalPrivateKey(priv), 0o600)
}

func saveState(dir string, st *State, certPEM []byte) error {
	b, _ := json.Marshal(st)
	if err := os.WriteFile(filepath.Join(dir, certFile), certPEM, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, stateFile), b, 0o600)
}

func loadState(dir string) (*State, tls.Certificate, error) {
	b, err := os.ReadFile(filepath.Join(dir, stateFile)) //nolint:gosec // fixed name inside the data dir
	if err != nil {
		return nil, tls.Certificate{}, ErrNotEnrolled
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, tls.Certificate{}, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, keyFile)) //nolint:gosec // fixed name inside the data dir
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	certPEM, err := os.ReadFile(filepath.Join(dir, certFile)) //nolint:gosec // fixed name inside the data dir
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	return &st, pair, err
}

// saveStateOnly rewrites state.json (the certificate is not touched). The file is replaced atomically.
func saveStateOnly(dir string, st *State) error {
	b, _ := json.Marshal(st)
	p := filepath.Join(dir, stateFile)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
