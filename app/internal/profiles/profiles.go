// Package profiles keeps the servers the application knows: the address, the pinned fingerprint and a name. The
// file holds nothing secret (a fingerprint is public); a saved password lives in the system keychain.
package profiles

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Profile is a server the person connected to.
type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"` // host:port of the application port
	Fingerprint string `json:"fingerprint"`
	BasePath    string `json:"base_path,omitempty"`
	Login       string `json:"login,omitempty"` // the last login, to fill the form
}

type file struct {
	Last     string    `json:"last"`
	Profiles []Profile `json:"profiles"`
}

// Store is the profiles file.
type Store struct {
	path string
	mu   sync.Mutex
	f    file
}

// ErrNotFound means there is no profile with this id.
var ErrNotFound = errors.New("profile not found")

// Open reads the file (a missing file is an empty store).
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path) //nolint:gosec // the path is the application's own config file
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.f); err != nil {
		return nil, err
	}
	return s, nil
}

// DefaultPath is <user config dir>/Linager/profiles.json.
func DefaultPath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "Linager", "profiles.json"), nil
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s.f, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// List returns the profiles, the last used first.
func (s *Store) List() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, 0, len(s.f.Profiles))
	for _, p := range s.f.Profiles {
		if p.ID == s.f.Last {
			out = append([]Profile{p}, out...)
		} else {
			out = append(out, p)
		}
	}
	return out
}

// Get returns a profile.
func (s *Store) Get(id string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.f.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, ErrNotFound
}

// Put adds a profile or replaces the one with the same address (one profile per server) and makes it the last used.
func (s *Store) Put(p Profile) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, old := range s.f.Profiles {
		if old.Address == p.Address {
			p.ID = old.ID
			if p.Name == "" {
				p.Name = old.Name
			}
			if p.Login == "" {
				p.Login = old.Login
			}
			s.f.Profiles[i] = p
			s.f.Last = p.ID
			return p, s.save()
		}
	}
	if p.ID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		p.ID = hex.EncodeToString(b)
	}
	s.f.Profiles = append(s.f.Profiles, p)
	s.f.Last = p.ID
	return p, s.save()
}

// SetLogin remembers the login of a profile.
func (s *Store) SetLogin(id, login string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.f.Profiles {
		if s.f.Profiles[i].ID == id {
			s.f.Profiles[i].Login = login
			return s.save()
		}
	}
	return ErrNotFound
}

// Delete removes a profile.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.f.Profiles[:0]
	for _, p := range s.f.Profiles {
		if p.ID != id {
			out = append(out, p)
		}
	}
	s.f.Profiles = out
	if s.f.Last == id {
		s.f.Last = ""
	}
	return s.save()
}
