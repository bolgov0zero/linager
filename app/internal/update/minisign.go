// Package update finds, downloads, verifies and installs a new version of the application. The releases are the ones the
// server and the agent come from: a signed manifest.json (minisign) lists every file with its sha256 and size, so a
// file that does not match what the owner signed is never installed.
package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// PublicKey is the release signing key (a minisign public key line), the same one the server and the agent trust.
var PublicKey = "RWQlhVj7fNoehSMCgQigmYrwQMjs/DyxnH1zLVEXkNXINxSTC0HDdCOU"

// Key is a parsed minisign public key.
type Key struct {
	ID  [8]byte
	Pub ed25519.PublicKey
}

// ParsePublicKey accepts the bare base64 line or a whole .pub file.
func ParsePublicKey(s string) (*Key, error) {
	line := ""
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "untrusted comment:") {
			line = l
			break
		}
	}
	b, err := base64.StdEncoding.DecodeString(line)
	if err != nil || len(b) != 42 || b[0] != 'E' || b[1] != 'd' {
		return nil, errors.New("invalid minisign public key")
	}
	k := &Key{Pub: ed25519.PublicKey(append([]byte(nil), b[10:]...))}
	copy(k.ID[:], b[2:10])
	return k, nil
}

// Verify checks a minisign signature file (.minisig) over message, including the trusted comment.
func (k *Key) Verify(message []byte, sigFile string) error {
	lines := strings.Split(strings.ReplaceAll(sigFile, "\r\n", "\n"), "\n")
	if len(lines) < 4 || !strings.HasPrefix(lines[0], "untrusted comment:") || !strings.HasPrefix(lines[2], "trusted comment:") {
		return errors.New("malformed signature file")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(sig) != 74 {
		return errors.New("malformed signature")
	}
	if !bytes.Equal(sig[2:10], k.ID[:]) {
		return errors.New("signature was made with a different key")
	}
	body := message
	switch string(sig[:2]) {
	case "Ed":
	case "ED":
		h := blake2b.Sum512(message)
		body = h[:]
	default:
		return errors.New("unsupported signature algorithm")
	}
	if !ed25519.Verify(k.Pub, body, sig[10:]) {
		return errors.New("signature verification failed")
	}
	trusted := strings.TrimPrefix(lines[2], "trusted comment:")
	trusted = strings.TrimPrefix(trusted, " ")
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(global) != ed25519.SignatureSize {
		return errors.New("malformed trusted-comment signature")
	}
	if !ed25519.Verify(k.Pub, append(append([]byte(nil), sig[10:]...), trusted...), global) {
		return errors.New("trusted comment verification failed")
	}
	return nil
}
