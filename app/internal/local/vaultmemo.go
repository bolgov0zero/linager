package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// vaultTTL is how long the application remembers the file of the vault: as long as the sign-in to the server lasts.
// Every use renews it, like the session.
const vaultTTL = 7 * 24 * time.Hour

// vaultMemo remembers which file the person chose for the Keys, so that it need not be picked again. Only the path is
// kept (never the passphrase) and the page cannot write it: it is not a preference.
type vaultMemo struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
}

type vaultRec struct {
	Path string `json:"path"`
	At   int64  `json:"at"`
}

func newVaultMemo(path string) *vaultMemo { return &vaultMemo{path: path, now: time.Now} }

func (m *vaultMemo) remember(file string) {
	if m == nil || m.path == "" || file == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, _ := json.Marshal(vaultRec{Path: file, At: m.now().Unix()})
	if os.MkdirAll(filepath.Dir(m.path), 0o700) != nil {
		return
	}
	_ = os.WriteFile(m.path, b, 0o600)
}

// recall returns the remembered file when it is not older than the period; reading renews the period.
func (m *vaultMemo) recall() (string, bool) {
	if m == nil || m.path == "" {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, err := os.ReadFile(m.path) //nolint:gosec // the application's own file
	if err != nil {
		return "", false
	}
	var r vaultRec
	if json.Unmarshal(b, &r) != nil || r.Path == "" {
		return "", false
	}
	if m.now().Sub(time.Unix(r.At, 0)) > vaultTTL {
		_ = os.Remove(m.path)
		return "", false
	}
	if nb, err := json.Marshal(vaultRec{Path: r.Path, At: m.now().Unix()}); err == nil {
		_ = os.WriteFile(m.path, nb, 0o600)
	}
	return r.Path, true
}

func (m *vaultMemo) forget() {
	if m == nil || m.path == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = os.Remove(m.path)
}
