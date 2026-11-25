package server

import (
	"testing"

	"github.com/bolgov0zero/linager/internal/pki"
)

// The two certificates have two fingerprints, and each is the one of its own DER bytes: a client that gets the ECDSA certificate
// (an iPhone) must find its fingerprint where the server shows it.
func TestKeysFingerprints(t *testing.T) {
	k := &Keys{TLSDER: []byte("ed25519 certificate"), TLSAltDER: []byte("ecdsa certificate")}
	fp, alt := k.Fingerprints()
	if fp != pki.Fingerprint(k.TLSDER) || alt != pki.Fingerprint(k.TLSAltDER) {
		t.Fatalf("fingerprints %q %q do not match the certificates", fp, alt)
	}
	if fp == alt {
		t.Fatal("the two certificates must have different fingerprints")
	}
}
