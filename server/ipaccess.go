package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/bolgov0zero/linager/server/store"
)

// Access restriction of the application port: a white list (what is not listed is refused) or a black list (what is not
// listed is allowed) of addresses, networks and host names, judged by the address of the connection. The port is a direct
// TLS connection by design, there is no proxy in front of it and the forwarding headers are dropped, so the address of the
// connection is the address of the client. The lists are set in the application (Settings, "Server access").

const (
	// legacyAccessSetting is where the restriction of the admin panel (removed with the panel) was stored; its lists are
	// carried to the application port once, so that a port that was closed stays closed.
	legacyAccessSetting = "access"

	// AccessOff, AccessWhite and AccessBlack are the modes of the restriction.
	AccessOff   = "off"
	AccessWhite = "white"
	AccessBlack = "black"

	// accessRetentionDays is how long the journal of the connections is kept.
	accessRetentionDays = 30
	maxAccessRules      = 500
	maxRuleNote         = 100

	denyLogEvery = time.Minute // the same address is refused: one journal row per this time
	maxLogSeen   = 4096
	reloadEvery  = 5 * time.Second
)

// AccessRule is one address, network or host name of a list.
type AccessRule struct {
	CIDR string `json:"cidr"`
	Note string `json:"note"`
}

// accessRules is the compiled form of a config.
type accessRules struct {
	mode           string
	white, black   []netip.Prefix
	whiteN, blackN int
	hosts          []string // the host names of both lists, sorted
	unresolved     []string // the host names that have no address in force
}

// ParseAccessRule reads "203.0.113.7", "203.0.113.0/24" or an IPv6 form and returns the normalised rule.
func ParseAccessRule(r AccessRule) (AccessRule, netip.Prefix, error) {
	s := strings.TrimSpace(r.CIDR)
	var p netip.Prefix
	if strings.Contains(s, "/") {
		var err error
		if p, err = netip.ParsePrefix(s); err != nil {
			return r, p, fmt.Errorf("bad address %q", s)
		}
		if p.Addr().Is4In6() {
			return r, p, fmt.Errorf("bad address %q", s)
		}
		p = p.Masked()
	} else {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return r, p, fmt.Errorf("bad address %q", s)
		}
		a = a.Unmap()
		p = netip.PrefixFrom(a, a.BitLen())
	}
	note, err := cleanNote(r.Note)
	if err != nil {
		return r, p, err
	}
	return AccessRule{CIDR: p.String(), Note: note}, p, nil
}

func cleanNote(s string) (string, error) {
	note := strings.TrimSpace(s)
	if utf8.RuneCountInString(note) > maxRuleNote {
		return "", errors.New("comment is too long")
	}
	return note, nil
}

// compileRules normalises a list. With allowHosts an entry that is not an address may be a host name; the names come
// back separately (their addresses are known only to the guard), the networks are the listed addresses.
func compileRules(in []AccessRule, limit int, allowHosts bool) ([]AccessRule, []netip.Prefix, []string, error) {
	if len(in) > limit {
		return nil, nil, nil, errors.New("too many entries")
	}
	rules := make([]AccessRule, 0, len(in))
	nets := make([]netip.Prefix, 0, len(in))
	var hosts []string
	seen := map[string]bool{}
	for _, r := range in {
		nr, p, err := ParseAccessRule(r)
		isHost := false
		if err != nil {
			h, ok := normalizeHost(r.CIDR)
			if !ok {
				return nil, nil, nil, err
			}
			if !allowHosts {
				return nil, nil, nil, errors.New("a host name is not allowed here, give an address")
			}
			note, nerr := cleanNote(r.Note)
			if nerr != nil {
				return nil, nil, nil, nerr
			}
			nr, isHost = AccessRule{CIDR: h, Note: note}, true
		}
		if seen[nr.CIDR] {
			continue
		}
		seen[nr.CIDR] = true
		rules = append(rules, nr)
		if isHost {
			hosts = append(hosts, nr.CIDR)
		} else {
			nets = append(nets, p)
		}
	}
	return rules, nets, hosts, nil
}

func hostPrefixes(addrs []netip.Addr) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out
}

func inNets(nets []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap()
	for _, n := range nets {
		if n.Contains(a) {
			return true
		}
	}
	return false
}

// Reason codes of a verdict (the interface translates them).
const (
	reasonOff        = "off"
	reasonInWhite    = "in_white"
	reasonNotInWhite = "not_in_white"
	reasonInBlack    = "in_black"
	reasonNotInBlack = "not_in_black"
	reasonBadAddress = "bad_address"
)

// verdict is the decision about one connection.
type verdict struct {
	IP      string `json:"ip"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// decide judges the address of a connection.
func (ar *accessRules) decide(peer netip.Addr) verdict {
	peer = peer.Unmap()
	v := verdict{IP: peer.String()}
	if ar.mode == AccessOff {
		v.Allowed, v.Reason = true, reasonOff
		return v
	}
	if !peer.IsValid() {
		v.Reason = reasonBadAddress
		return v
	}
	if ar.mode == AccessWhite {
		if v.Allowed = inNets(ar.white, peer); v.Allowed {
			v.Reason = reasonInWhite
		} else {
			v.Reason = reasonNotInWhite
		}
		return v
	}
	if inNets(ar.black, peer) {
		v.Reason = reasonInBlack
		return v
	}
	v.Allowed, v.Reason = true, reasonNotInBlack
	return v
}

// accessProblem is a reason why a config must not be applied (the interface translates the code).
type accessProblem struct {
	Code   string `json:"code"`
	IP     string `json:"ip,omitempty"`
	Reason string `json:"reason,omitempty"`
	Host   string `json:"host,omitempty"`
}

// Problem codes.
const (
	problemWhiteEmpty = "white_empty"
	problemLockout    = "lockout"
	problemHostNoAddr = "host_unresolved"
)

// AccessGuard holds the restriction in force, the host names of its lists and writes the journal of the connections.
type AccessGuard struct {
	db  *store.Store
	log *slog.Logger
	app atomic.Pointer[appAccessSnapshot] // nil: nothing is saved, the port is open

	mu       sync.Mutex
	lastDeny map[string]time.Time

	applyMu sync.Mutex // the snapshot is replaced by one writer at a time

	hmu          sync.Mutex // the host names: what is known about each, and what was stored last
	hosts        map[string]*hostEntry
	persistedKey string
	persistedAt  time.Time
}

func newAccessGuard(db *store.Store, log *slog.Logger) *AccessGuard {
	return &AccessGuard{db: db, log: log, lastDeny: map[string]time.Time{}, hosts: map[string]*hostEntry{}}
}

// Load reads the stored restriction.
func (g *AccessGuard) Load(ctx context.Context) {
	g.migrateLegacy(ctx)
	g.loadApp(ctx)
}

// migrateLegacy carries the lists of the removed admin panel restriction to the application port, once: until the port has
// lists of its own, the port followed that restriction, so an upgrade must not open a port that was closed.
func (g *AccessGuard) migrateLegacy(ctx context.Context) {
	if len(g.db.SettingRaw(ctx, appAccessSetting)) != 0 {
		return
	}
	raw := g.db.SettingRaw(ctx, legacyAccessSetting)
	if len(raw) == 0 {
		return
	}
	var old struct {
		Mode  string       `json:"mode"`
		White []AccessRule `json:"white"`
		Black []AccessRule `json:"black"`
	}
	if json.Unmarshal(raw, &old) != nil || (old.Mode != AccessWhite && old.Mode != AccessBlack) {
		return
	}
	cfg := AppAccessConfig{Mode: old.Mode, White: old.White, Black: old.Black}
	norm, _, err := compileAppAccess(cfg, nil)
	if err != nil {
		g.log.Warn("access restriction: the lists of the admin panel cannot be carried to the application port", "err", err)
		return
	}
	b, _ := json.Marshal(norm)
	if err := g.db.SetSettingRaw(ctx, appAccessSetting, b); err != nil {
		g.log.Warn("access restriction: carrying the lists of the admin panel failed", "err", err)
		return
	}
	g.log.Info("access restriction: the lists of the admin panel were carried to the application port", "mode", norm.Mode)
}

// Run reloads the restriction every few seconds (so that `linager access reset` takes effect without a restart), looks the
// host names up and removes the old journal rows.
func (g *AccessGuard) Run(ctx context.Context) {
	go g.hostLoop(ctx)
	reload := time.NewTicker(reloadEvery)
	purge := time.NewTicker(10 * time.Minute)
	defer reload.Stop()
	defer purge.Stop()
	g.Purge(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-reload.C:
			g.Load(ctx)
		case <-purge.C:
			g.Purge(ctx)
		}
	}
}

// Purge removes the journal rows older than the retention and everything above the row cap.
func (g *AccessGuard) Purge(ctx context.Context) {
	if err := g.db.PurgeAccessLog(ctx, accessRetentionDays*24*time.Hour); err != nil && ctx.Err() == nil {
		g.log.Warn("purge access log", "err", err)
	}
}

// ResetAccess turns the restriction of the application port off in the database (the lists stay). It is the way back when
// the administrator has locked themselves out: `linager access reset` on the server.
func ResetAccess(ctx context.Context, db *store.Store) (bool, error) {
	changed, err := resetApp(ctx, db)
	if err != nil {
		return false, err
	}
	if changed {
		_ = db.WriteAudit(ctx, store.Audit{Actor: "cli", Action: "access.reset", TargetType: "access", Result: "ok"})
	}
	return changed, nil
}

// ---- the journal ----

func (g *AccessGuard) record(r *http.Request, v verdict, peer netip.Addr, event, login, reason string) {
	if g.db == nil { // tests of the guard alone
		return
	}
	row := store.AccessLogRow{ClientIP: v.IP, RemoteAddr: peer.String(), Event: event, Login: login, Path: r.URL.Path, UserAgent: r.UserAgent(), Reason: reason}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer cancel()
	if err := g.db.InsertAccessLog(ctx, row); err != nil {
		g.log.Error("access log write failed", "event", event, "err", err)
	}
}

// due reports whether a row about this address may be written now and remembers it.
func (g *AccessGuard) due(seen map[string]time.Time, key string, every time.Duration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if t, ok := seen[key]; ok && now.Sub(t) < every {
		return false
	}
	if len(seen) >= maxLogSeen {
		for k, t := range seen {
			if now.Sub(t) >= every {
				delete(seen, k)
			}
		}
		if len(seen) >= maxLogSeen {
			clear(seen)
		}
	}
	seen[key] = now
	return true
}

// logLogin writes a sign-in attempt (the result is the audit result: ok, failed, locked, rate_limited).
func (g *AccessGuard) logLogin(r *http.Request, login, result string) {
	event, reason := store.AccessLoginOK, ""
	if result != "ok" {
		event, reason = store.AccessLoginFail, result
	}
	peer := remoteAddr(r).Unmap()
	g.record(r, verdict{IP: peer.String(), Allowed: true}, peer, event, login, reason)
}

// ---- the guard in front of the API ----

func (a *Admin) accessGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := a.Access
		peer := remoteAddr(r).Unmap()
		v := g.decideApp(peer)
		if !v.Allowed {
			if g.due(g.lastDeny, v.IP, denyLogEvery) {
				g.record(r, v, peer, store.AccessDenied, "", v.Reason)
			}
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
