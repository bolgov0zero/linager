package profiles

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Prefs keeps the choices of the person that the interface makes (theme, language, the layout of the dashboard,
// the section). A page of the application lives on a random address, so the storage of the web view cannot keep
// them from one start to the next: they are kept in a file instead. Nothing secret goes here.
type Prefs struct {
	path string
	mu   sync.Mutex
	m    map[string]string
}

const (
	maxPrefs     = 64
	maxPrefValue = 4096
)

// OpenPrefs reads the file (a missing file is an empty set; a damaged one is dropped).
func OpenPrefs(path string) *Prefs {
	p := &Prefs{path: path, m: map[string]string{}}
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // the path is the application's own config file
		var m map[string]string
		if json.Unmarshal(b, &m) == nil && m != nil {
			p.m = m
		}
	}
	return p
}

// PrefsPath is prefs.json next to the profiles file.
func PrefsPath(profilesPath string) string {
	return filepath.Join(filepath.Dir(profilesPath), "prefs.json")
}

// All returns a copy of the choices.
func (p *Prefs) All() map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]string, len(p.m))
	for k, v := range p.m {
		out[k] = v
	}
	return out
}

// Merge sets the given choices (an empty value removes one) and writes the file.
func (p *Prefs) Merge(in map[string]string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := make(map[string]string, len(p.m)+len(in))
	for k, v := range p.m {
		next[k] = v
	}
	for k, v := range in {
		if k == "" || len(k) > 64 || len(v) > maxPrefValue {
			return errors.New("bad preference")
		}
		if v == "" {
			delete(next, k)
		} else {
			next[k] = v
		}
	}
	if len(next) > maxPrefs {
		return errors.New("too many preferences")
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(next, "", "  ")
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.path); err != nil {
		return err
	}
	p.m = next
	return nil
}
