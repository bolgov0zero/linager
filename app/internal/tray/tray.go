// Package tray keeps what the tray of the application shows: the connections the person put there (a terminal, the remote
// desktop or the Assistant of a server) and a short state of the servers. Building the menu is separate from drawing it
// (main.go), so it can be tested.
package tray

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Modes are the connections a server can have in the tray.
var Modes = []string{"terminal", "rdp", "asst"}

// Entry is a server with the connections that are in the tray.
type Entry struct {
	ServerID string   `json:"server_id"`
	Name     string   `json:"name"`
	Modes    []string `json:"modes"`
}

// Store keeps the entries in a file, per profile (the list of servers belongs to one Linager server).
type Store struct {
	path string
	mu   sync.Mutex
	m    map[string][]Entry
}

// Open reads the file (a missing or damaged file is an empty set).
func Open(path string) *Store {
	s := &Store{path: path, m: map[string][]Entry{}}
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // the application's own file
		var m map[string][]Entry
		if json.Unmarshal(b, &m) == nil && m != nil {
			s.m = m
		}
	}
	return s
}

// List returns the entries of a profile, sorted by name.
func (s *Store) List(profile string) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Entry(nil), s.m[profile]...)
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func validMode(m string) bool {
	for _, x := range Modes {
		if x == m {
			return true
		}
	}
	return false
}

// Set puts the connections of a server in the tray; an empty list takes the server out.
func (s *Store) Set(profile string, e Entry) error {
	if profile == "" || e.ServerID == "" || len(e.ServerID) > 64 {
		return errors.New("bad entry")
	}
	modes := make([]string, 0, len(e.Modes))
	for _, m := range Modes { // in a fixed order, without repeats
		for _, x := range e.Modes {
			if x == m {
				modes = append(modes, m)
				break
			}
		}
	}
	for _, x := range e.Modes {
		if !validMode(x) {
			return errors.New("unknown connection")
		}
	}
	e.Modes = modes
	if len(e.Name) > 200 {
		e.Name = e.Name[:200]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Entry, 0, len(s.m[profile])+1)
	for _, x := range s.m[profile] {
		if x.ServerID != e.ServerID {
			list = append(list, x)
		}
	}
	if len(modes) > 0 {
		list = append(list, e)
	}
	if len(list) > 100 {
		return errors.New("too many servers in the tray")
	}
	next := make(map[string][]Entry, len(s.m)+1)
	for k, v := range s.m {
		next[k] = v
	}
	if len(list) == 0 {
		delete(next, profile)
	} else {
		next[profile] = list
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(next, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.m = next
	return nil
}

// Rename updates the name of a server in the entries (the name may have changed on the server).
func (s *Store) Rename(profile, serverID, name string) {
	s.mu.Lock()
	var e *Entry
	for i := range s.m[profile] {
		if s.m[profile][i].ServerID == serverID && s.m[profile][i].Name != name {
			c := s.m[profile][i]
			e = &c
		}
	}
	s.mu.Unlock()
	if e != nil {
		e.Name = name
		_ = s.Set(profile, *e)
	}
}

// Status is the short state of the servers shown at the top of the menu.
type Status struct {
	Total     int `json:"total"`
	Online    int `json:"online"`
	Attention int `json:"attention"` // online servers with a problem (the thresholds of the dashboard)
	Waiting   int `json:"waiting"`   // agents that ask to be approved
}

// Summarize reads the answer of api/dashboard (the same thresholds as levelOf of the interface).
func Summarize(dashboard []byte) (Status, error) {
	var d struct {
		Items []struct {
			Online bool `json:"online"`
			Sample *struct {
				CPU            float64 `json:"cpu"`
				MemPct         float64 `json:"mem_pct"`
				DiskPct        float64 `json:"disk_pct"`
				FailedServices int     `json:"failed_services"`
				RebootRequired bool    `json:"reboot_required"`
				Updates        int     `json:"updates"`
			} `json:"sample"`
		} `json:"items"`
	}
	if err := json.Unmarshal(dashboard, &d); err != nil {
		return Status{}, err
	}
	st := Status{Total: len(d.Items)}
	for _, it := range d.Items {
		if !it.Online {
			continue
		}
		st.Online++
		if s := it.Sample; s != nil {
			if s.CPU > 75 || s.MemPct > 85 || s.DiskPct > 80 || s.FailedServices > 0 || s.RebootRequired || s.Updates >= 10 {
				st.Attention++
			}
		}
	}
	return st, nil
}

// Item is a row of the menu.
type Item struct {
	Kind     string // open, status, sep, conn, quit
	Label    string
	Mode     string // conn: terminal, rdp, asst (the icon and the window)
	ServerID string
	Name     string
	Disabled bool
}

var words = map[string]map[string]string{
	"ru": {"open": "Открыть", "quit": "Закрыть", "status": "В сети %d из %d", "attention": ", внимание: %d", "waiting": ", ждут: %d"},
	"en": {"open": "Open", "quit": "Quit", "status": "Online %d of %d", "attention": ", attention: %d", "waiting": ", waiting: %d"},
}

// Menu builds the menu: "Open" first, then the state, the connections and "Quit" last. status is nil when it is not known
// (not signed in).
func Menu(lang string, status *Status, entries []Entry) []Item {
	w := words[lang]
	if w == nil {
		w = words["en"]
	}
	items := []Item{{Kind: "open", Label: w["open"]}, {Kind: "sep"}}
	if status != nil {
		label := sprintf(w["status"], status.Online, status.Total)
		if status.Attention > 0 {
			label += sprintf(w["attention"], status.Attention)
		}
		if status.Waiting > 0 {
			label += sprintf(w["waiting"], status.Waiting)
		}
		items = append(items, Item{Kind: "status", Label: label, Disabled: true}, Item{Kind: "sep"})
	}
	n := 0
	for _, e := range entries {
		for _, m := range e.Modes {
			items = append(items, Item{Kind: "conn", Label: e.Name, Mode: m, ServerID: e.ServerID, Name: e.Name})
			n++
		}
	}
	if n > 0 {
		items = append(items, Item{Kind: "sep"})
	}
	return append(items, Item{Kind: "quit", Label: w["quit"]})
}
