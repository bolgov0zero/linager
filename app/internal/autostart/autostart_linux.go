package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

func entryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "linager.desktop"), nil
}

// Enabled says whether the application starts at login.
func Enabled() bool {
	p, err := entryPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Set turns the start at login on or off; exe is the running program.
func Set(on bool, exe string) error {
	p, err := entryPath()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(desktopFile(exe)), 0o644) //nolint:gosec // a .desktop file is read by the session, it holds no secret
}
