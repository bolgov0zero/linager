package profiles

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// Secrets keeps the passwords the person asked to remember.
type Secrets interface {
	Get(profile string) (string, error)
	Set(profile, password string) error
	Delete(profile string) error
}

// ErrNoSecret means nothing is saved for the profile.
var ErrNoSecret = errors.New("no saved password")

const service = "Linager"

// Keychain is the system keychain: Keychain on macOS, Credential Manager on Windows, Secret Service on Linux.
type Keychain struct{}

// Get returns the saved password.
func (Keychain) Get(profile string) (string, error) {
	v, err := keyring.Get(service, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNoSecret
	}
	return v, err
}

// Set saves a password.
func (Keychain) Set(profile, password string) error { return keyring.Set(service, profile, password) }

// Delete forgets a password; forgetting what is not there is not an error.
func (Keychain) Delete(profile string) error {
	err := keyring.Delete(service, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Memory is a Secrets in memory, for tests and for the headless run.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

// Get returns the saved password.
func (s *Memory) Get(profile string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[profile]; ok {
		return v, nil
	}
	return "", ErrNoSecret
}

// Set saves a password.
func (s *Memory) Set(profile, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[profile] = password
	return nil
}

// Delete forgets a password.
func (s *Memory) Delete(profile string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, profile)
	return nil
}
