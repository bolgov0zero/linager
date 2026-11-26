package profiles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceTokenIsMadeOnceAndKept(t *testing.T) {
	s, dir := &Memory{}, t.TempDir()
	a, err := DeviceToken(s, dir, "p1")
	if err != nil || !validToken(a) {
		t.Fatalf("%q %v", a, err)
	}
	if b, _ := DeviceToken(s, dir, "p1"); b != a {
		t.Fatal("the same profile has the same device")
	}
	if c, _ := DeviceToken(s, dir, "p2"); c == a {
		t.Fatal("another server sees another device")
	}
	ForgetDeviceToken(s, dir, "p1")
	if d, _ := DeviceToken(s, dir, "p1"); d == a {
		t.Fatal("a forgotten profile is a new device")
	}
	if _, err := DeviceToken(s, dir, ""); err == nil {
		t.Fatal("no profile, no token")
	}
}

// brokenKeychain is a keychain that cannot be written (no secret service).
type brokenKeychain struct{ Memory }

func (*brokenKeychain) Set(string, string) error { return errors.New("no secret service") }

func TestDeviceTokenFallsBackToAFileWithTheRights600(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	s := &brokenKeychain{}
	a, err := DeviceToken(s, dir, "p/1")
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "device-p_1"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", st, err)
	}
	if b, _ := DeviceToken(s, dir, "p/1"); b != a {
		t.Fatal("the file keeps it between runs")
	}
	ForgetDeviceToken(s, dir, "p/1")
	if _, err := os.Stat(filepath.Join(dir, "device-p_1")); err == nil {
		t.Fatal("forgotten")
	}
	if strings.ContainsAny(safeName("a/b\\c:d"), "/\\:") {
		t.Fatal("safe name")
	}
}
