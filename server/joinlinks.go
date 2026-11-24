package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/internal/version"
	"github.com/bolgov0zero/linager/server/store"
	"github.com/bolgov0zero/linager/update"
)

// An install link carries a secret token. Only its hash is stored; the secret is shown once, when it is made.
const joinPrefix = "lk_"

var joinSecretRe = regexp.MustCompile(`^lk_[A-Za-z0-9_-]{22}$`)

// newJoinSecret makes a token: 128 random bits. The label is the short form that the panel lists.
func newJoinSecret() (secret, label string, hash []byte, err error) {
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return "", "", nil, err
	}
	secret = joinPrefix + base64.RawURLEncoding.EncodeToString(b)
	return secret, joinLabel(secret), joinHash(secret), nil
}

func joinLabel(secret string) string {
	if len(secret) < 8 {
		return secret
	}
	return secret[:7] + "…" + secret[len(secret)-2:]
}

func joinHash(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// spkiPin is the value of the --pinnedpubkey option of wget and curl for the certificate: the SHA-256 of its public key.
func spkiPin(der []byte) (string, error) {
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return "sha256//" + base64.StdEncoding.EncodeToString(sum[:]), nil
}

// parseNetworks turns "10.0.0.0/8, 192.168.1.5" into prefixes; a bare address is a /32 (or /128).
func parseNetworks(in []string) ([]string, error) {
	var out []string
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			p, err := netip.ParsePrefix(part)
			if err != nil {
				a, aerr := netip.ParseAddr(part)
				if aerr != nil {
					return nil, fmt.Errorf("%q is not an address or a network", part)
				}
				p = netip.PrefixFrom(a, a.BitLen())
			}
			out = append(out, p.Masked().String())
		}
	}
	if len(out) > 20 {
		return nil, errors.New("at most 20 networks")
	}
	return out, nil
}

func inNetworks(ip netip.Addr, nets []string) bool {
	if len(nets) == 0 {
		return true
	}
	ip = ip.Unmap()
	for _, n := range nets {
		if p, err := netip.ParsePrefix(n); err == nil && p.Contains(ip) {
			return true
		}
	}
	return false
}

// lookupJoin finds a token by its secret and checks that it may be used from ip. Every failure looks the same.
func (a *AgentPort) lookupJoin(ctx context.Context, secret string, ip netip.Addr) (*store.JoinToken, bool) {
	if !joinSecretRe.MatchString(secret) {
		return nil, false
	}
	t, err := a.DB.JoinTokenByHash(ctx, joinHash(secret))
	if err != nil || !t.Valid(time.Now()) || !inNetworks(ip, t.Networks) {
		return nil, false
	}
	return t, true
}

// joinToken is the common start of the two link routes: rate limit, secret, validity, source network.
func (a *AgentPort) joinToken(w http.ResponseWriter, r *http.Request) (*store.JoinToken, bool) {
	ip := remoteAddr(r)
	if !a.joinL.Allow(ip.String()) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return nil, false
	}
	t, ok := a.lookupJoin(r.Context(), r.PathValue("token"), ip)
	if !ok {
		http.NotFound(w, r)
		return nil, false
	}
	return t, true
}

// joinBinary serves the verified release binary of this server to a host that came with a valid link.
func (a *AgentPort) joinBinary(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.joinToken(w, r); !ok {
		return
	}
	if a.Updates == nil {
		http.Error(w, "releases are not available", http.StatusServiceUnavailable)
		return
	}
	c, err := a.Updates.CurrentRelease(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	name, ok := releaseBinary(c.Manifest, r.URL.Query().Get("arch"))
	if !ok {
		http.Error(w, "this release has no binary for the requested architecture", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, filepath.Join(c.Dir, name)) //nolint:gosec // a verified cache entry and a name from its manifest
}

// joinScript serves the shell script that the install command runs. It is plain text a person can read first.
func (a *AgentPort) joinScript(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.joinToken(w, r); !ok {
		return
	}
	pin, err := spkiPin(a.Keys.TLSDER)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// the checksums come from the cached release only: the script must not wait for a download
	hashes := map[string]string{}
	if a.Updates != nil {
		if c, err := update.LoadCached(a.Updates.Key, a.Updates.Cache, version.Version); err == nil {
			for arch, as := range c.Manifest.AssetList() {
				hashes[arch] = as.SHA256
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprint(w, joinScriptText(a.serverAddr(r), pki.Fingerprint(a.Keys.TLSDER), pin, r.PathValue("token"), hashes))
}

var hostRe = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)

// serverAddr is host:port as the client used it, so the script connects the same way; it falls back to the
// configured listen port when the Host header carries none.
func (a *AgentPort) serverAddr(r *http.Request) string {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = strings.Trim(r.Host, "[]"), a.ListenPort()
	}
	if ip, perr := netip.ParseAddr(host); perr == nil {
		host = ip.String()
	} else if !hostRe.MatchString(host) {
		host = "localhost"
	}
	if port == "" {
		port = a.ListenPort()
	}
	return net.JoinHostPort(host, port)
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// joinScriptText is what `wget ... | sh` runs on the new host. It checks the system, takes the binary from
// this server over the pinned connection, compares it with the signed manifest and starts the installer, which
// pairs the agent with the token.
func joinScriptText(server, fp, pin, token string, hashes map[string]string) string {
	var sums strings.Builder
	for _, arch := range []string{"amd64", "arm64"} {
		if h := hashes[arch]; h != "" {
			fmt.Fprintf(&sums, "  %s) want=%s ;;\n", arch, shq(h))
		}
	}
	return fmt.Sprintf(`#!/bin/sh
# Linager: install the agent by a link made in the application.
# The script comes from your Linager server; read it before you run it.
set -eu

SERVER=%s
FP=%s
PIN=%s
TOKEN=%s

die() { printf 'Error: %%s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
command -v wget >/dev/null 2>&1 || die "wget is required"
[ -r /etc/os-release ] || die "cannot detect the OS"
os_id=$(. /etc/os-release && printf '%%s' "${ID:-}")
os_ver=$(. /etc/os-release && printf '%%s' "${VERSION_ID:-}")
case "$os_id:$os_ver" in
  debian:12|debian:13|ubuntu:22.04|ubuntu:24.04|ubuntu:26.04) ;;
  *) die "unsupported OS: $os_id $os_ver (Debian 12/13, Ubuntu 22.04/24.04/26.04)" ;;
esac
arch=$(dpkg --print-architecture 2>/dev/null || true)
want=
case "$arch" in
%s  amd64|arm64) ;;
  *) die "unsupported architecture: $arch (amd64 and arm64 only)" ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Downloading the agent from %%s ...\n' "$SERVER"
wget -nv --no-check-certificate --pinnedpubkey "$PIN" -O "$tmp/linager" "https://$SERVER/v1/join/$TOKEN/binary?arch=$arch" || die "download failed"
if [ -n "$want" ]; then
  got=$(sha256sum "$tmp/linager" | cut -d' ' -f1)
  [ "$got" = "$want" ] || die "the binary does not match the signed release"
fi
chmod 0755 "$tmp/linager"

"$tmp/linager" install agent --no-tui --server "$SERVER" --fingerprint "$FP" --join-token "$TOKEN"

# The downloaded copy is no longer needed (the installer put the binary into /usr/local/bin); exec skips the trap.
cd /
rm -rf "$tmp"
trap - EXIT INT TERM

# A person at a terminal gets the menu next, as if they had typed linager. With "| sh" the standard input is the
# pipe, so the keyboard is taken from /dev/tty. Automation without a terminal ends here.
if [ -t 1 ] && (: </dev/tty) 2>/dev/null; then
  case "${LC_ALL:-${LC_MESSAGES:-${LANG:-}}}" in
    ru*) hint='Готово. Нажмите Enter, чтобы открыть меню Linager (то же самое откроет команда linager)...' ;;
    *) hint='Done. Press Enter to open the Linager menu (the same as typing linager)...' ;;
  esac
  printf '\n%%s ' "$hint"
  read -r _ </dev/tty || exit 0
  exec /usr/local/bin/linager </dev/tty
fi
`, shq(server), shq(fp), shq(pin), shq(token), sums.String())
}

// ---- the addresses that the commands use ----

// hostKind sorts an address or a name of the server into "ext" (seen from the internet), "loc" (the local
// network) or "" (not usable from another host).
func hostKind(h string) string {
	h = strings.ToLower(strings.Trim(h, "[]"))
	if h == "" || h == "localhost" {
		return ""
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		switch {
		case ip.IsLoopback(), ip.IsUnspecified(), ip.IsLinkLocalUnicast(), ip.IsMulticast():
			return ""
		case ip.IsPrivate(), netip.MustParsePrefix("100.64.0.0/10").Contains(ip):
			return "loc"
		}
		return "ext"
	}
	if !strings.Contains(h, ".") {
		return "loc" // a bare host name ("homeSRV") resolves only inside the local network
	}
	for _, suf := range []string{".local", ".lan", ".home", ".internal", ".home.arpa", ".localdomain"} {
		if strings.HasSuffix(h, suf) {
			return "loc"
		}
	}
	return "ext"
}

// localAddr is a private IPv4 address of this machine, for the local command when the configuration names none.
func localAddr() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(ipn.IP); ok && ip.Unmap().Is4() && ip.Unmap().IsPrivate() {
				return ip.Unmap().String()
			}
		}
	}
	return ""
}

// joinHosts picks the two addresses of the commands from the names of the agent-port certificate and from the
// address the administrator opened the panel with.
func joinHosts(cfg *config.Server, reqHost string) (ext, loc string) {
	cand := append([]string(nil), cfg.Agents.Hosts...)
	if h, _, err := net.SplitHostPort(reqHost); err == nil {
		cand = append(cand, h)
	} else {
		cand = append(cand, reqHost)
	}
	locIP := false
	for _, h := range cand {
		switch hostKind(h) {
		case "ext":
			if ext == "" {
				ext = strings.Trim(h, "[]")
			}
		case "loc":
			// an address of the local network is more reliable than a name (Windows does not resolve bare names)
			if _, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil && !locIP {
				loc, locIP = strings.Trim(h, "[]"), true
			} else if loc == "" {
				loc = strings.Trim(h, "[]")
			}
		}
	}
	if loc == "" {
		loc = localAddr()
	}
	return ext, loc
}

func fingerprintOf(der []byte) string { return pki.Fingerprint(der) }

// SPKIPin is spkiPin for tests in other packages.
func SPKIPin(der []byte) (string, error) { return spkiPin(der) }
