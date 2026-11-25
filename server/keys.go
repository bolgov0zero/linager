package server

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"

	"github.com/bolgov0zero/linager/internal/pki"
)

// Keys is the server key material kept in the data directory.
type Keys struct {
	CA     *pki.CA
	TLS    tls.Certificate
	TLSDER []byte
	// TLSAlt is the same server with an ECDSA key, for clients that cannot verify Ed25519 (Windows PowerShell).
	TLSAlt    tls.Certificate
	TLSAltDER []byte
	CmdKey    ed25519.PrivateKey
	CmdPubkey ed25519.PublicKey
}

func writeSecret(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

// Fingerprints are the fingerprints of the two server certificates: the Ed25519 one and the ECDSA one. Which of them a client sees
// depends on what it can verify: Go and wget get the first, Apple's TLS (iPhone, iPad) and Windows PowerShell get the second. A person who
// compares what a client shows with what the server shows needs the one the client got.
func (k *Keys) Fingerprints() (primary, alt string) {
	return pki.Fingerprint(k.TLSDER), pki.Fingerprint(k.TLSAltDER)
}

// InitKeys creates missing keys (CA, agent-port TLS certificate, command signing key).
// Existing files are never overwritten.
func InitKeys(dir string, hosts []string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.key")); errors.Is(err, os.ErrNotExist) {
		ca, err := pki.NewCA("Linager CA")
		if err != nil {
			return err
		}
		if err := writeSecret(filepath.Join(dir, "ca.crt"), ca.CertPEM); err != nil {
			return err
		}
		if err := writeSecret(filepath.Join(dir, "ca.key"), ca.KeyPEM()); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "agents.key")); errors.Is(err, os.ErrNotExist) {
		c, k, err := pki.NewServerCert(hosts)
		if err != nil {
			return err
		}
		if err := writeSecret(filepath.Join(dir, "agents.crt"), c); err != nil {
			return err
		}
		if err := writeSecret(filepath.Join(dir, "agents.key"), k); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "agents-ec.key")); errors.Is(err, os.ErrNotExist) {
		c, k, err := pki.NewServerCertECDSA(hosts)
		if err != nil {
			return err
		}
		if err := writeSecret(filepath.Join(dir, "agents-ec.crt"), c); err != nil {
			return err
		}
		if err := writeSecret(filepath.Join(dir, "agents-ec.key"), k); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "cmd.key")); errors.Is(err, os.ErrNotExist) {
		_, priv, err := pki.GenerateKey()
		if err != nil {
			return err
		}
		return writeSecret(filepath.Join(dir, "cmd.key"), pki.MarshalPrivateKey(priv))
	}
	return nil
}

// LoadKeys reads the key material created by InitKeys.
func LoadKeys(dir string) (*Keys, error) {
	read := func(n string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, n)) } //nolint:gosec // fixed names in the data dir
	caCrt, err := read("ca.crt")
	if err != nil {
		return nil, err
	}
	caKey, err := read("ca.key")
	if err != nil {
		return nil, err
	}
	ca, err := pki.LoadCA(caCrt, caKey)
	if err != nil {
		return nil, err
	}
	crt, err := read("agents.crt")
	if err != nil {
		return nil, err
	}
	key, err := read("agents.key")
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(crt, key)
	if err != nil {
		return nil, err
	}
	// the ECDSA certificate was added later: a server that was installed before gets it here, for the same hosts
	if _, err := os.Stat(filepath.Join(dir, "agents-ec.key")); errors.Is(err, os.ErrNotExist) {
		leaf, lerr := x509.ParseCertificate(pair.Certificate[0])
		if lerr != nil {
			return nil, lerr
		}
		hosts := append([]string{}, leaf.DNSNames...)
		for _, ip := range leaf.IPAddresses {
			hosts = append(hosts, ip.String())
		}
		c, k, gerr := pki.NewServerCertECDSA(hosts)
		if gerr != nil {
			return nil, gerr
		}
		if werr := writeSecret(filepath.Join(dir, "agents-ec.crt"), c); werr != nil {
			return nil, werr
		}
		if werr := writeSecret(filepath.Join(dir, "agents-ec.key"), k); werr != nil {
			return nil, werr
		}
	}
	altCrt, err := read("agents-ec.crt")
	if err != nil {
		return nil, err
	}
	altKey, err := read("agents-ec.key")
	if err != nil {
		return nil, err
	}
	alt, err := tls.X509KeyPair(altCrt, altKey)
	if err != nil {
		return nil, err
	}
	cmdPEM, err := read("cmd.key")
	if err != nil {
		return nil, err
	}
	cmd, err := pki.ParsePrivateKey(cmdPEM)
	if err != nil {
		return nil, err
	}
	return &Keys{CA: ca, TLS: pair, TLSDER: pair.Certificate[0], TLSAlt: alt, TLSAltDER: alt.Certificate[0], CmdKey: cmd, CmdPubkey: cmd.Public().(ed25519.PublicKey)}, nil
}
