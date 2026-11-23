package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d", "profiles.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Put(Profile{Name: "a", Address: "a:1", Fingerprint: "SHA256:a"})
	if err != nil || a.ID == "" {
		t.Fatalf("put: %+v %v", a, err)
	}
	b, _ := s.Put(Profile{Name: "b", Address: "b:1", Fingerprint: "SHA256:b"})
	if l := s.List(); len(l) != 2 || l[0].ID != b.ID {
		t.Fatalf("the last used goes first: %+v", l)
	}
	// the same address replaces (a new pin after the person confirmed a new certificate) and keeps the id and the login
	_ = s.SetLogin(a.ID, "admin")
	a2, _ := s.Put(Profile{Address: "a:1", Fingerprint: "SHA256:new"})
	if a2.ID != a.ID || a2.Fingerprint != "SHA256:new" || a2.Name != "a" || a2.Login != "admin" || len(s.List()) != 2 {
		t.Fatalf("replace: %+v", a2)
	}
	// it survives a restart, and the file is private
	s2, err := Open(path)
	if err != nil || len(s2.List()) != 2 {
		t.Fatalf("reopen: %v %+v", err, s2.List())
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the file must be 600: %v %v", fi, err)
	}
	if err := s2.Delete(a.ID); err != nil || len(s2.List()) != 1 {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s2.Get(a.ID); err != ErrNotFound {
		t.Fatalf("get after delete: %v", err)
	}
}

func TestMemorySecrets(t *testing.T) {
	var m Memory
	if _, err := m.Get("p"); err != ErrNoSecret {
		t.Fatal(err)
	}
	_ = m.Set("p", "x")
	if v, _ := m.Get("p"); v != "x" {
		t.Fatal(v)
	}
	_ = m.Delete("p")
	if _, err := m.Get("p"); err != ErrNoSecret {
		t.Fatal(err)
	}
}
