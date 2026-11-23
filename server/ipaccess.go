package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/bolgov0zero/linager/server/store"
)

// Access restriction of the admin panel: a white list (what is not listed is refused) or a black list (what is not
// listed is allowed), judged by the real client address. The address comes from X-Forwarded-For only when the
// connection is made by a trusted proxy; otherwise it is the address of the connection itself, so the clients of a
// local network can reach the panel directly while the internet comes through the proxy.

const (
	accessSetting = "access"

	// AccessOff, AccessWhite and AccessBlack are the modes of the restriction.
	AccessOff   = "off"
	AccessWhite = "white"
	AccessBlack = "black"

	defaultAccessRetention = 30
	maxAccessRules         = 500
	maxAccessProxies       = 50
	maxRuleNote            = 100

	pageLogEvery = 10 * time.Minute // the same address opens the page: one journal row per this time
	denyLogEvery = time.Minute      // the same address is refused: one journal row per this time
	maxLogSeen   = 4096
	reloadEvery  = 5 * time.Second
)

// AccessRule is one address or network of a list.
type AccessRule struct {
	CIDR string `json:"cidr"`
	Note string `json:"note"`
}

// AccessConfig is the restriction as the admin edits and stores it.
type AccessConfig struct {
	Mode      string       `json:"mode"`
	OnlyProxy bool         `json:"only_proxy"`
	White     []AccessRule `json:"white"`
	Black     []AccessRule `json:"black"`
	Proxies   []AccessRule `json:"proxies"`
	Retention int          `json:"retention_days"`
}

// accessRules is the compiled form of the config.
type accessRules struct {
	mode            string
	onlyProxy       bool
	white, black    []netip.Prefix
	proxies         []netip.Prefix
	whiteN, blackN  int
	proxiesN        int
	trustAllProxies bool
	hosts           []string // the host names of both lists, sorted
	unresolved      []string // the host names that have no address in force
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

// compileAccess validates and normalises a config. hosts holds the addresses in force for the host names of the
// lists (nil: none are known, which is fine for validation only).
func compileAccess(cfg AccessConfig, hosts hostAddrs) (AccessConfig, *accessRules, error) {
	switch cfg.Mode {
	case AccessOff, AccessWhite, AccessBlack:
	default:
		return cfg, nil, errors.New("unknown mode")
	}
	if cfg.Retention == 0 {
		cfg.Retention = defaultAccessRetention
	}
	if cfg.Retention < 1 || cfg.Retention > 365 {
		return cfg, nil, errors.New("retention must be 1 to 365 days")
	}
	var err error
	ar := &accessRules{mode: cfg.Mode, onlyProxy: cfg.OnlyProxy}
	var whiteHosts, blackHosts []string
	if cfg.White, ar.white, whiteHosts, err = compileRules(cfg.White, maxAccessRules, true); err != nil {
		return cfg, nil, err
	}
	if cfg.Black, ar.black, blackHosts, err = compileRules(cfg.Black, maxAccessRules, true); err != nil {
		return cfg, nil, err
	}
	if cfg.Proxies, ar.proxies, _, err = compileRules(cfg.Proxies, maxAccessProxies, false); err != nil {
		return cfg, nil, err
	}
	all := map[string]bool{}
	for _, h := range append(append([]string{}, whiteHosts...), blackHosts...) {
		all[h] = true
	}
	if len(all) > maxAccessHosts {
		return cfg, nil, errors.New("too many host names")
	}
	for h := range all {
		ar.hosts = append(ar.hosts, h)
	}
	sort.Strings(ar.hosts)
	for _, h := range ar.hosts {
		if len(hosts[h]) == 0 {
			ar.unresolved = append(ar.unresolved, h)
		}
	}
	for _, h := range whiteHosts {
		ar.white = append(ar.white, hostPrefixes(hosts[h])...)
	}
	for _, h := range blackHosts {
		ar.black = append(ar.black, hostPrefixes(hosts[h])...)
	}
	ar.whiteN, ar.blackN, ar.proxiesN = len(cfg.White), len(cfg.Black), len(cfg.Proxies)
	for _, p := range ar.proxies {
		if p.Bits() == 0 {
			ar.trustAllProxies = true
		}
	}
	return cfg, ar, nil
}

// Reason codes of a verdict (the interface translates them).
const (
	reasonOff        = "off"
	reasonOnlyProxy  = "only_proxy"
	reasonInWhite    = "in_white"
	reasonNotInWhite = "not_in_white"
	reasonInBlack    = "in_black"
	reasonNotInBlack = "not_in_black"
	reasonBadAddress = "bad_address"
)

// verdict is the decision about one connection.
type verdict struct {
	IP         string `json:"ip"`
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
	ViaProxy   bool   `json:"via_proxy"`   // the connection is made by a trusted proxy
	IgnoredXFF bool   `json:"ignored_xff"` // X-Forwarded-For came from an address that is not a trusted proxy
}

func listed(nets []netip.Prefix, a netip.Addr) bool {
	for _, n := range nets {
		if n.Contains(a) {
			return true
		}
	}
	return false
}

// decideDirect judges a connection to the application port. That port is a direct TLS connection by design, there is
// no proxy in front of it, so "only through a trusted proxy" cannot apply to it; the white and black lists still
// judge the real address of the connection (the forwarding headers are dropped on that port).
func (ar *accessRules) decideDirect(peer netip.Addr) verdict {
	d := *ar
	d.onlyProxy = false
	return d.decide(peer, "")
}

// decide judges a connection by the peer address and the X-Forwarded-For header.
func (ar *accessRules) decide(peer netip.Addr, xff string) verdict {
	peerTrusted := peer.IsValid() && inNets(ar.proxies, peer)
	ip := effectiveIP(ar.proxies, peer, xff)
	v := verdict{IP: ip, ViaProxy: peerTrusted, IgnoredXFF: xff != "" && !peerTrusted}
	if ar.mode == AccessOff {
		v.Allowed, v.Reason = true, reasonOff
		return v
	}
	if ar.onlyProxy && !peerTrusted {
		v.Reason = reasonOnlyProxy
		return v
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		v.Reason = reasonBadAddress
		return v
	}
	a = a.Unmap()
	if ar.mode == AccessWhite {
		if v.Allowed = listed(ar.white, a); v.Allowed {
			v.Reason = reasonInWhite
		} else {
			v.Reason = reasonNotInWhite
		}
		return v
	}
	if listed(ar.black, a) {
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
	problemTrustAll     = "trust_all"
	problemProxyUnknown = "proxy_unlisted"
	problemWhiteEmpty   = "white_empty"
	problemLockout      = "lockout"
	problemHostNoAddr   = "host_unresolved"
)

// problems lists what is wrong with enabling the config for the request that is changing it. An empty answer
// means the admin keeps access and the address that is judged cannot be forged.
func (ar *accessRules) problems(peer netip.Addr, xff string) []accessProblem {
	out := []accessProblem{}
	if ar.mode == AccessOff {
		return out
	}
	if ar.trustAllProxies {
		out = append(out, accessProblem{Code: problemTrustAll})
	}
	if xff != "" && peer.IsValid() && !inNets(ar.proxies, peer) {
		out = append(out, accessProblem{Code: problemProxyUnknown, IP: peer.String()})
	}
	if ar.mode == AccessWhite && ar.whiteN == 0 {
		out = append(out, accessProblem{Code: problemWhiteEmpty})
	}
	for _, h := range ar.unresolved {
		out = append(out, accessProblem{Code: problemHostNoAddr, Host: h})
	}
	if v := ar.decide(peer, xff); !v.Allowed {
		out = append(out, accessProblem{Code: problemLockout, IP: v.IP, Reason: v.Reason})
	}
	return out
}

// accessSnapshot is the config in force.
type accessSnapshot struct {
	cfg        AccessConfig
	rules      *accessRules
	configured bool // false: nothing is stored yet, the proxies come from server.yaml
}

// AccessGuard holds the restriction in force and writes the journal of visits.
type AccessGuard struct {
	db    *store.Store
	log   *slog.Logger
	trust *ProxyTrust
	yaml  []string
	snap  atomic.Pointer[accessSnapshot]

	mu       sync.Mutex
	lastPage map[string]time.Time
	lastDeny map[string]time.Time

	applyMu sync.Mutex // the snapshot is replaced by one writer at a time

	hmu          sync.Mutex // the host names: what is known about each, and what was stored last
	hosts        map[string]*hostEntry
	persistedKey string
	persistedAt  time.Time
}

func newAccessGuard(db *store.Store, trust *ProxyTrust, yamlProxies []string, log *slog.Logger) *AccessGuard {
	g := &AccessGuard{db: db, log: log, trust: trust, yaml: yamlProxies, lastPage: map[string]time.Time{}, lastDeny: map[string]time.Time{}, hosts: map[string]*hostEntry{}}
	g.apply(g.defaults(), false)
	return g
}

// defaults is the config of an installation where nothing is stored: no restriction, proxies from server.yaml.
func (g *AccessGuard) defaults() AccessConfig {
	cfg := AccessConfig{Mode: AccessOff, Retention: defaultAccessRetention, White: []AccessRule{}, Black: []AccessRule{}, Proxies: []AccessRule{}}
	for _, c := range g.yaml {
		cfg.Proxies = append(cfg.Proxies, AccessRule{CIDR: c, Note: ""})
	}
	return cfg
}

func (g *AccessGuard) apply(cfg AccessConfig, configured bool) {
	g.applyMu.Lock()
	defer g.applyMu.Unlock()
	g.applyLocked(cfg, configured)
}

func (g *AccessGuard) applyLocked(cfg AccessConfig, configured bool) {
	hosts := g.addrs()
	norm, rules, err := compileAccess(cfg, hosts)
	if err != nil {
		// a damaged stored value must not lock everybody out: fall back to no restriction and say so
		g.log.Error("access restriction: stored settings are invalid, the restriction is off", "err", err)
		norm, rules, _ = compileAccess(g.defaults(), hosts)
		configured = false
	}
	g.trust.Set(rules.proxies)
	g.snap.Store(&accessSnapshot{cfg: norm, rules: rules, configured: configured})
}

// reapply compiles the config in force again, for the new addresses of its host names.
func (g *AccessGuard) reapply() {
	g.applyMu.Lock()
	defer g.applyMu.Unlock()
	s := g.snap.Load()
	g.applyLocked(s.cfg, s.configured)
}

// Load reads the stored restriction.
func (g *AccessGuard) Load(ctx context.Context) {
	raw := g.db.SettingRaw(ctx, accessSetting)
	if len(raw) == 0 {
		g.apply(g.defaults(), false)
		return
	}
	cfg := AccessConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		g.log.Error("access restriction: stored settings are unreadable, the restriction is off", "err", err)
		g.apply(g.defaults(), false)
		return
	}
	g.apply(cfg, true)
}

// Save stores a validated config and puts it in force.
func (g *AccessGuard) Save(ctx context.Context, cfg AccessConfig) error {
	norm, _, err := compileAccess(cfg, nil)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(norm)
	if err != nil {
		return err
	}
	if err := g.db.SetSettingRaw(ctx, accessSetting, raw); err != nil {
		return err
	}
	g.apply(norm, true)
	g.persistHosts(ctx, g.snap.Load().rules.hosts)
	return nil
}

// Current returns the config in force.
func (g *AccessGuard) Current() (AccessConfig, bool) {
	s := g.snap.Load()
	return s.cfg, s.configured
}

// Run reloads the restriction every few seconds (so that `linager access reset` takes effect without a restart)
// and removes the old journal rows.
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

// Purge removes the journal rows older than the retention.
func (g *AccessGuard) Purge(ctx context.Context) {
	cfg, _ := g.Current()
	if err := g.db.PurgeAccessLog(ctx, time.Duration(cfg.Retention)*24*time.Hour); err != nil && ctx.Err() == nil {
		g.log.Warn("purge access log", "err", err)
	}
}

// ResetAccess turns the restriction off in the database (the lists stay). It is the way back when the admin has
// locked themselves out: `linager access reset` on the server.
func ResetAccess(ctx context.Context, db *store.Store) (bool, error) {
	raw := db.SettingRaw(ctx, accessSetting)
	if len(raw) == 0 {
		return false, nil
	}
	cfg := AccessConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		cfg = AccessConfig{}
	}
	changed := (cfg.Mode != AccessOff && cfg.Mode != "") || cfg.OnlyProxy
	cfg.Mode, cfg.OnlyProxy = AccessOff, false
	norm, _, err := compileAccess(cfg, nil)
	if err != nil {
		return false, err
	}
	out, _ := json.Marshal(norm)
	if err := db.SetSettingRaw(ctx, accessSetting, out); err != nil {
		return false, err
	}
	_ = db.WriteAudit(ctx, store.Audit{Actor: "cli", Action: "access.reset", TargetType: "access", Result: "ok"})
	return changed, nil
}

// ---- the journal ----

func (g *AccessGuard) record(r *http.Request, v verdict, peer netip.Addr, event, login, reason string) {
	if g.db == nil { // tests of the guard alone
		return
	}
	row := store.AccessLogRow{ClientIP: v.IP, RemoteAddr: peer.String(), ViaProxy: r.Header.Get("X-Forwarded-For") != "", ProxyTrusted: v.ViaProxy,
		Event: event, Login: login, Path: r.URL.Path, UserAgent: r.UserAgent(), Reason: reason}
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
func (g *AccessGuard) logLogin(r *http.Request, v verdict, login, result string) {
	event, reason := store.AccessLoginOK, ""
	if result != "ok" {
		event, reason = store.AccessLoginFail, result
	}
	g.record(r, v, remoteAddr(r), event, login, reason)
}

// ---- the guard in front of the admin panel ----

// isPageOpen tells the request that opens the panel itself (not a background request of the interface).
func (a *Admin) isPageOpen(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	p := strings.TrimPrefix(r.URL.Path, strings.TrimRight(a.Cfg.Admin.BasePath, "/"))
	return (p == "" || p == "/" || p == "/index.html") && strings.Contains(r.Header.Get("Accept"), "text/html")
}

// isLocalProbe tells the health probe of the updater: a plain GET of the panel root made from this very host,
// directly (no X-Forwarded-For) and not by a browser (no text/html in Accept). The updater of the running
// version, which may be an older one, asks the new server this way whether it came up; without the exception
// every restriction that does not list 127.0.0.1 would make the update roll back with "403 Forbidden". The
// answer is the static page shell, nothing else is reachable this way, and every other path stays guarded.
func (a *Admin) isLocalProbe(r *http.Request, peer netip.Addr) bool {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !peer.IsValid() || !peer.Unmap().IsLoopback() {
		return false
	}
	if r.Header.Get("X-Forwarded-For") != "" || strings.Contains(r.Header.Get("Accept"), "text/html") {
		return false
	}
	base := strings.TrimRight(a.Cfg.Admin.BasePath, "/")
	return r.URL.Path == base+"/" || (base != "" && r.URL.Path == base)
}

func (a *Admin) accessGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := a.Access
		peer, xff := remoteAddr(r), r.Header.Get("X-Forwarded-For")
		if a.isLocalProbe(r, peer) {
			next.ServeHTTP(w, r)
			return
		}
		rules := g.snap.Load().rules
		var v verdict
		if isDirect(r) {
			v = rules.decideDirect(peer)
		} else {
			v = rules.decide(peer, xff)
		}
		if !v.Allowed {
			if g.due(g.lastDeny, v.IP, denyLogEvery) {
				g.record(r, v, peer, store.AccessDenied, "", v.Reason)
			}
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if a.isPageOpen(r) && g.due(g.lastPage, v.IP, pageLogEvery) {
			g.record(r, v, peer, store.AccessPage, "", "")
		}
		next.ServeHTTP(w, r)
	})
}

type ctxDirect struct{}

// markDirect tells the access guard that the request came to the application port, not through a proxy.
func markDirect(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxDirect{}, true))
}

func isDirect(r *http.Request) bool {
	v, _ := r.Context().Value(ctxDirect{}).(bool)
	return v
}
