package local

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// sumOf is the fingerprint of the bytes of a vault file: the page keeps the fingerprint of what it read or wrote last, and a
// write in place is refused when the file on the disk is not that any more (it was changed by someone else).
func sumOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// writeVaultInPlace replaces the file with the new bytes so that a break in the middle cannot leave half a file: the bytes go
// to a temporary file in the same folder (readable by its owner only), which is then renamed over the file. The previous
// content stays beside it as `<file>.bak` (one copy, the one before the last write).
func writeVaultInPlace(path string, old, data []byte) error {
	if err := os.WriteFile(path+".bak", old, 0o600); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lvault-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
