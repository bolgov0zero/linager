package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

func agentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", ID+".plist"), nil
}

// Enabled says whether the application starts at login.
func Enabled() bool {
	p, err := agentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Set turns the start at login on or off; exe is the running program.
func Set(on bool, exe string) error {
	p, err := agentPath()
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
	return os.WriteFile(p, []byte(plist(exe)), 0o644) //nolint:gosec // a LaunchAgent is read by launchd, it holds no secret
}
