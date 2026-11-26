package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/bolgov0zero/linager/server/store"
)

// Live fans server-sent events out to connected browsers.
type Live struct {
	mu   sync.Mutex
	subs map[chan liveMsg]struct{}
}

// liveMsg is one event; server is the server it is about ("" for events that concern nobody in particular).
type liveMsg struct {
	server string
	b      []byte
	// admin: only administrators get it (the devices waiting for approval and the like).
	admin bool
}

// NewLive creates an empty fan-out.
func NewLive() *Live { return &Live{subs: map[chan liveMsg]struct{}{}} }

// Publish sends one named event to every subscriber; slow subscribers drop events.
func (l *Live) Publish(event string, data any) { l.PublishServer("", event, data) }

// PublishServer sends an event about one server; a subscriber that may not see that server does not get it.
func (l *Live) PublishServer(serverID, event string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := liveMsg{server: serverID, b: []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, b))}
	l.mu.Lock()
	defer l.mu.Unlock()
	for ch := range l.subs {
		select {
		case ch <- msg:
		default: // subscriber is too slow; the browser reloads state on reconnect
		}
	}
}

// PublishAdmin sends an event that only administrators may get.
func (l *Live) PublishAdmin(event string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	l.send(liveMsg{b: []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, b)), admin: true})
}

func (l *Live) send(msg liveMsg) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for ch := range l.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (l *Live) subscribe() (chan liveMsg, func()) {
	ch := make(chan liveMsg, 256)
	l.mu.Lock()
	l.subs[ch] = struct{}{}
	l.mu.Unlock()
	return ch, func() {
		l.mu.Lock()
		delete(l.subs, ch)
		l.mu.Unlock()
	}
}

// handler streams events to one authenticated browser.
func (a *Admin) liveHandler(w http.ResponseWriter, r *http.Request, u *store.User) {
	fl, ok := w.(http.Flusher)
	if !ok {
		jsonErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	ch, unsub := a.Live.subscribe()
	defer unsub()
	// an operator or an auditor only gets the events of the servers they may see (the list is refreshed now and then)
	var visible map[string]bool
	var loaded time.Time
	allow := func(id string) bool {
		if u.Role == store.RoleAdmin {
			return true
		}
		if visible == nil || time.Since(loaded) > 20*time.Second {
			v, err := a.DB.AllowedServerIDs(r.Context(), u.ID)
			if err != nil {
				return false
			}
			visible, loaded = v, time.Now()
		}
		return visible[id]
	}
	_, _ = w.Write([]byte("retry: 3000\n\n"))
	fl.Flush()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-ch:
			if m.admin && u.Role != store.RoleAdmin {
				continue
			}
			if m.server != "" && !allow(m.server) {
				continue
			}
			if _, err := w.Write(m.b); err != nil {
				return
			}
			fl.Flush()
		case <-tick.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
