package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"sync"

	"github.com/coder/websocket"

	"github.com/bolgov0zero/linager/internal/protocol"
)

// conn is one connected agent.
type conn struct {
	ws      *websocket.Conn
	key     ed25519.PrivateKey
	mu      sync.Mutex
	seq     uint64
	cancel  context.CancelFunc
	version string
	state   *connState
}

func (c *conn) send(ctx context.Context, typ string, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	env, err := protocol.Sign(c.key, typ, c.seq, payload)
	if err != nil {
		return err
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return c.ws.Write(ctx, websocket.MessageText, b)
}

// Hub tracks connected agents by server id.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*conn
	// OnChange is called (without locks held) when an agent connects or disconnects.
	OnChange func(serverID string, online bool)
}

// NewHub creates an empty hub.
func NewHub() *Hub { return &Hub{conns: map[string]*conn{}} }

func (h *Hub) add(id string, c *conn) {
	h.mu.Lock()
	old := h.conns[id]
	h.conns[id] = c
	h.mu.Unlock()
	if old != nil {
		old.cancel()
		_ = old.ws.Close(websocket.StatusPolicyViolation, "replaced by a new connection")
	} else if h.OnChange != nil {
		h.OnChange(id, true)
	}
}

func (h *Hub) remove(id string, c *conn) {
	h.mu.Lock()
	removed := h.conns[id] == c
	if removed {
		delete(h.conns, id)
	}
	h.mu.Unlock()
	if removed && h.OnChange != nil {
		h.OnChange(id, false)
	}
}

// Online reports whether the agent is connected.
func (h *Hub) Online(id string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conns[id] != nil
}

// OnlineIDs lists the connected agents.
func (h *Hub) OnlineIDs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.conns))
	for id := range h.conns {
		ids = append(ids, id)
	}
	return ids
}

// Send signs and sends a command to a connected agent.
func (h *Hub) Send(ctx context.Context, id, typ string, payload any) error {
	h.mu.RLock()
	c := h.conns[id]
	h.mu.RUnlock()
	if c == nil {
		return errors.New("agent is offline")
	}
	return c.send(ctx, typ, payload)
}

// Kick closes an agent connection (used after a server is deleted).
func (h *Hub) Kick(id string) {
	h.mu.Lock()
	c := h.conns[id]
	delete(h.conns, id)
	h.mu.Unlock()
	if c != nil {
		c.cancel()
		_ = c.ws.Close(websocket.StatusPolicyViolation, "server removed")
	}
}
