// Package protocol defines the signed message envelope exchanged between server and agents.
package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Version is the current protocol version; MinVersion is the oldest the server accepts.
const (
	Version    = 1
	MinVersion = 1
)

// Message types.
const (
	TypeHello       = "hello"
	TypePing        = "ping"
	TypePong        = "pong"
	TypeAgentRemove = "agent.uninstall"
	// TypeCertRenew is sent by an agent whose certificate is close to expiry; the server answers
	// with TypeCertRenewed. Both sides ignore the messages when they do not know them.
	TypeCertRenew   = "cert.renew"
	TypeCertRenewed = "cert.renewed"
	// TypeServerMove tells an agent that the agent port of the server changed. The agent tries the new port
	// and falls back to the old one when it does not work. An agent that does not know the message ignores it.
	TypeServerMove = "server.move"
)

// MoveMinAgent is the first agent version that follows TypeServerMove.
const MoveMinAgent = "0.10.0-beta.1"

// Values of the X-Linager-Reason header on a refused agent connection.
const (
	ReasonRevoked = "revoked" // the server no longer knows this agent
	ReasonExpired = "expired" // the agent certificate is outside its validity period
)

// CertRenewed carries a freshly issued agent certificate.
type CertRenewed struct {
	CertPEM string `json:"cert_pem"`
}

// ServerMove names the new port of the agent listener; the host stays the one the agent already uses.
type ServerMove struct {
	Port int `json:"port"`
}

// MaxSkew is how far a message timestamp may differ from the local clock.
const MaxSkew = 2 * time.Minute

// Envelope is one signed message.
type Envelope struct {
	V       int             `json:"v"`
	ID      string          `json:"id"`
	Seq     uint64          `json:"seq"`
	TS      int64           `json:"ts"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Sig     []byte          `json:"sig"`
}

// Hello is the payload of the first message in each direction.
type Hello struct {
	AgentVersion string   `json:"agent_version,omitempty"`
	OS           string   `json:"os,omitempty"`
	Hostname     string   `json:"hostname,omitempty"`
	Modules      []string `json:"modules,omitempty"`
	MinProtocol  int      `json:"min_protocol,omitempty"`
}

// Uninstall asks the agent to remove itself from the host.
type Uninstall struct {
	Purge bool `json:"purge"` // also delete configuration and keys
}

func signingBytes(e *Envelope) []byte {
	return []byte(fmt.Sprintf("linager-v%d\n%s\n%s\n%d\n%d\n%s", e.V, e.Type, e.ID, e.Seq, e.TS, e.Payload))
}

// NewID returns a random message id.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Sign builds and signs an envelope.
func Sign(key ed25519.PrivateKey, typ string, seq uint64, payload any) (*Envelope, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	e := &Envelope{V: Version, ID: NewID(), Seq: seq, TS: time.Now().Unix(), Type: typ, Payload: raw}
	e.Sig = ed25519.Sign(key, signingBytes(e))
	return e, nil
}

// Verify checks version, signature and timestamp freshness.
func Verify(pub ed25519.PublicKey, e *Envelope, now time.Time) error {
	if e.V < MinVersion || e.V > Version {
		return fmt.Errorf("unsupported protocol version %d", e.V)
	}
	if len(e.Sig) != ed25519.SignatureSize || !ed25519.Verify(pub, signingBytes(e), e.Sig) {
		return errors.New("bad signature")
	}
	d := now.Sub(time.Unix(e.TS, 0))
	if d > MaxSkew || d < -MaxSkew {
		return errors.New("message timestamp out of range")
	}
	return nil
}

// SeqGuard rejects replayed or reordered messages on one connection.
type SeqGuard struct{ last uint64 }

// Accept returns an error unless seq is strictly greater than the last accepted one.
func (g *SeqGuard) Accept(seq uint64) error {
	if seq <= g.last {
		return fmt.Errorf("replayed or out-of-order message (seq %d <= %d)", seq, g.last)
	}
	g.last = seq
	return nil
}
