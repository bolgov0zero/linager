// Package release verifies minisign-compatible signatures and describes Linager releases.
package release

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// PublicKey is the release signing key (minisign public key line). It can be overridden at build time
// with -ldflags "-X github.com/bolgov0zero/linager/internal/release.PublicKey=RWQ...".
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

// Embedded returns the key compiled into the binary.
func Embedded() (*Key, error) { return ParsePublicKey(PublicKey) }

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

// ---- versions ----

// Version is a parsed semantic version.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
	Valid               bool
}

// ParseVersion parses "v1.2.3" or "1.2.3-beta.1". Anything else (for example "dev") is invalid.
func ParseVersion(s string) Version {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, pre, _ := strings.Cut(s, "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return Version{}
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2], pre, true}
}

// Compare returns -1, 0 or 1. Invalid versions sort below every valid one; a pre-release is below its release.
func Compare(a, b Version) int {
	switch {
	case !a.Valid && !b.Valid:
		return 0
	case !a.Valid:
		return -1
	case !b.Valid:
		return 1
	}
	for _, p := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	return comparePre(a.Pre, b.Pre)
}

// comparePre orders two pre-release labels as semantic versioning does: the dot-separated parts one by one, a number
// by its value (beta.9 is below beta.11), a number below a word, a word by its letters, and the label with fewer
// parts below one that begins the same way (beta is below beta.1).
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		xn, xerr := strconv.ParseUint(x, 10, 64)
		yn, yerr := strconv.ParseUint(y, 10, 64)
		switch {
		case xerr == nil && yerr == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case xerr == nil: // a number is below a word
			return -1
		case yerr == nil:
			return 1
		case x != y:
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

// Newer reports whether candidate is a higher version than current.
func Newer(candidate, current string) bool {
	return Compare(ParseVersion(candidate), ParseVersion(current)) > 0
}

// String renders v.
func (v Version) String() string {
	if !v.Valid {
		return "unknown"
	}
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// ---- manifest ----

// Manifest describes one release; it is signed as manifest.json (+ .minisig).
//
// SHA256 and Size describe the amd64 binary: they are what releases before arm64 support carried, and
// agents of those releases still read only them. Assets lists the binary of every architecture and is
// what newer servers and agents use.
type Manifest struct {
	Version    string           `json:"version"`
	Channel    string           `json:"channel"` // stable | beta
	MinAgent   string           `json:"min_agent"`
	SHA256     string           `json:"sha256"` // of linager_linux_amd64
	Size       int64            `json:"size"`
	Assets     map[string]Asset `json:"assets,omitempty"` // GOARCH -> binary
	Migrations int              `json:"migrations"`       // number of DB migrations the release ships
	Notes      []string         `json:"notes"`
	BuiltAt    string           `json:"built_at"`
}

// Asset is the binary of one architecture.
type Asset struct {
	Name   string `json:"name"` // BinaryAsset(arch)
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Architectures a release ships binaries for (GOARCH values).
const (
	ArchAMD64 = "amd64"
	ArchARM64 = "arm64"
)

// Arches lists the supported architectures.
var Arches = []string{ArchAMD64, ArchARM64}

// HostArch is the architecture of this process. Tests may override it.
var HostArch = runtime.GOARCH

// SupportedArch reports whether releases carry a binary for arch.
func SupportedArch(arch string) bool {
	for _, a := range Arches {
		if a == arch {
			return true
		}
	}
	return false
}

// WindowsKey is the key of the Windows (amd64) binary in Manifest.Assets. It is not a GOARCH value, so servers and
// agents of earlier releases, which read only the architectures they know, ignore the entry.
const WindowsKey = "windows_amd64"

// WindowsBinaryAsset is the release asset name of the Windows agent (the same file the MSI package installs).
const WindowsBinaryAsset = "linager-agent_windows_amd64.exe"

// WindowsMSIKey is the key of the Windows installer package in Manifest.Assets; the install by a link downloads it.
const WindowsMSIKey = "windows_msi"

// WindowsMSIAsset is the release asset name of the Windows installer package.
const WindowsMSIAsset = "linager-agent_windows_amd64.msi"

// WindowsMSI returns the installer package of the release, when it has one.
func (m Manifest) WindowsMSI() (Asset, bool) {
	a, ok := m.Assets[WindowsMSIKey]
	return a, ok && a.SHA256 != ""
}

// WindowsAsset returns the Windows binary of the release, when it has one.
func (m Manifest) WindowsAsset() (Asset, bool) {
	a, ok := m.Assets[WindowsKey]
	return a, ok && a.SHA256 != ""
}

// HostKey is the name this process asks the server for: the Windows key on Windows, the architecture elsewhere.
func HostKey() string {
	if runtime.GOOS == "windows" {
		return WindowsKey
	}
	return HostArch
}

// HostAsset returns the binary of the release for this process (see HostKey).
func (m Manifest) HostAsset() (Asset, bool) {
	if runtime.GOOS == "windows" {
		return m.WindowsAsset()
	}
	return m.Asset(HostArch)
}

// BinaryAsset is the release asset name of the binary for arch.
func BinaryAsset(arch string) string { return "linager_linux_" + arch }

// Asset returns the binary of arch. A manifest without an assets map (a release before arm64 support)
// has only the amd64 binary, described by the top-level fields.
func (m Manifest) Asset(arch string) (Asset, bool) {
	if !SupportedArch(arch) {
		return Asset{}, false
	}
	if a, ok := m.Assets[arch]; ok {
		return a, true
	}
	if arch == ArchAMD64 && m.SHA256 != "" {
		return Asset{Name: BinaryAsset(arch), SHA256: m.SHA256, Size: m.Size}, true
	}
	return Asset{}, false
}

// AssetList returns every binary of the release for the architectures this build knows. Entries for
// architectures it does not know are ignored, so a newer release does not block an older server.
func (m Manifest) AssetList() map[string]Asset {
	out := map[string]Asset{}
	for _, arch := range Arches {
		if a, ok := m.Asset(arch); ok {
			out[arch] = a
		}
	}
	return out
}

// Validate checks the parts of the manifest that decide which files are read and trusted.
func (m Manifest) Validate() error {
	if !ParseVersion(m.Version).Valid || len(m.SHA256) != 64 {
		return errors.New("manifest is invalid")
	}
	for arch, a := range m.AssetList() {
		if a.Name != BinaryAsset(arch) || len(a.SHA256) != 64 || a.Size < 0 {
			return fmt.Errorf("manifest: invalid %s asset", arch)
		}
	}
	return nil
}
