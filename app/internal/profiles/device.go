package profiles

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// The token of the device: a random secret that tells the server "this is the same installation of the application". It is made at the
// first connection to a server, kept in the system keychain (next to the passwords) and never shown. Where the keychain does not work
// (a Linux without a secret service) it is kept in a file of the configuration folder with the rights 600. One token for each profile:
// the same computer is a device of its own on each server.

const deviceKeyPrefix = "device:"

// DeviceToken returns the token of the device for a profile and makes it when it does not exist. dir is the folder of the fallback file.
func DeviceToken(secrets Secrets, dir, profileID string) (string, error) {
	if profileID == "" {
		return "", errors.New("no profile")
	}
	if v, err := secrets.Get(deviceKeyPrefix + profileID); err == nil && validToken(v) {
		return v, nil
	}
	file := filepath.Join(dir, "device-"+safeName(profileID))
	if dir != "" {
		if b, err := os.ReadFile(file); err == nil { //nolint:gosec // the application's own configuration folder
			if v := strings.TrimSpace(string(b)); validToken(v) {
				return v, nil
			}
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	if err := secrets.Set(deviceKeyPrefix+profileID, tok); err == nil {
		return tok, nil
	}
	if dir == "" {
		return "", errors.New("the token cannot be kept")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, []byte(tok+"\n"), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

// ForgetDeviceToken drops the token of a profile (the profile is forgotten: the server sees a new device next time).
func ForgetDeviceToken(secrets Secrets, dir, profileID string) {
	_ = secrets.Delete(deviceKeyPrefix + profileID)
	if dir != "" {
		_ = os.Remove(filepath.Join(dir, "device-"+safeName(profileID)))
	}
}

func validToken(v string) bool {
	if len(v) < 32 || len(v) > 128 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(v)
	return err == nil
}

// safeName keeps a profile id fit for a file name.
func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}
