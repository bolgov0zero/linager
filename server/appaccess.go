package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"sort"
)

// The lists of the application port: the white or the black list set in the application (Settings, "Server access"). Host
// names (a DDNS name of a home connection) are allowed: they are resolved by the job of ipaccess_hosts.go and judged as the
// addresses they point to.

const appAccessSetting = "app_access"

var errUnknownMode = errors.New("unknown mode")

// AppAccessConfig is the restriction of the application port as the administrator edits and stores it.
type AppAccessConfig struct {
	Mode  string       `json:"mode"`
	White []AccessRule `json:"white"`
	Black []AccessRule `json:"black"`
}

// appAccessSnapshot is the config in force; nil in the guard means "nothing saved: the port is open".
type appAccessSnapshot struct {
	cfg   AppAccessConfig
	rules *accessRules
}

// appHostNames lists the host names of the lists of a config (what is not an address but a name).
func appHostNames(cfg AppAccessConfig) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]AccessRule{cfg.White, cfg.Black} {
		for _, r := range list {
			if _, _, err := ParseAccessRule(r); err == nil {
				continue
			}
			if h, ok := normalizeHost(r.CIDR); ok && !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	sort.Strings(out)
	return out
}

// trackedHosts is every host name of the lists in force: the job that asks DNS looks up all of them.
func (g *AccessGuard) trackedHosts() []string {
	if s := g.app.Load(); s != nil {
		return append([]string(nil), s.rules.hosts...)
	}
	return nil
}

// reapplyApp compiles the list of the port again, for the new addresses of its host names.
func (g *AccessGuard) reapplyApp() {
	g.applyMu.Lock()
	defer g.applyMu.Unlock()
	s := g.app.Load()
	if s == nil {
		return
	}
	if norm, rules, err := compileAppAccess(s.cfg, g.addrs()); err == nil {
		g.app.Store(&appAccessSnapshot{cfg: norm, rules: rules})
	}
}

// compileAppAccess validates and normalises a config. hosts holds the addresses in force for the host names of the lists
// (nil: none are known, which is fine for validation only).
func compileAppAccess(cfg AppAccessConfig, hosts hostAddrs) (AppAccessConfig, *accessRules, error) {
	switch cfg.Mode {
	case AccessOff, AccessWhite, AccessBlack:
	default:
		return cfg, nil, errUnknownMode
	}
	ar := &accessRules{mode: cfg.Mode}
	var err error
	var whiteHosts, blackHosts []string
	if cfg.White, ar.white, whiteHosts, err = compileRules(cfg.White, maxAccessRules, true); err != nil {
		return cfg, nil, err
	}
	if cfg.Black, ar.black, blackHosts, err = compileRules(cfg.Black, maxAccessRules, true); err != nil {
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
	ar.whiteN, ar.blackN = len(cfg.White), len(cfg.Black)
	return cfg, ar, nil
}

// decideApp judges a connection to the application port.
func (g *AccessGuard) decideApp(peer netip.Addr) verdict {
	if s := g.app.Load(); s != nil {
		return s.rules.decide(peer)
	}
	return verdict{IP: peer.Unmap().String(), Allowed: true, Reason: reasonOff}
}

// AppCurrent is the restriction in force (off when nothing is saved).
func (g *AccessGuard) AppCurrent() AppAccessConfig {
	if s := g.app.Load(); s != nil {
		return s.cfg
	}
	return AppAccessConfig{Mode: AccessOff, White: []AccessRule{}, Black: []AccessRule{}}
}

func (g *AccessGuard) loadApp(ctx context.Context) {
	raw := g.db.SettingRaw(ctx, appAccessSetting)
	if len(raw) == 0 {
		g.app.Store(nil)
		return
	}
	cfg := AppAccessConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		g.log.Error("application access restriction: stored settings are unreadable, the restriction is off", "err", err)
		cfg = AppAccessConfig{Mode: AccessOff}
	}
	norm, rules, err := compileAppAccess(cfg, g.addrs())
	if err != nil {
		g.log.Error("application access restriction: stored settings are invalid, the restriction is off", "err", err)
		norm, rules, _ = compileAppAccess(AppAccessConfig{Mode: AccessOff}, nil)
	}
	g.app.Store(&appAccessSnapshot{cfg: norm, rules: rules})
}

// SaveApp stores a validated config and puts it in force.
func (g *AccessGuard) SaveApp(ctx context.Context, cfg AppAccessConfig) error {
	g.applyMu.Lock()
	defer g.applyMu.Unlock()
	norm, rules, err := compileAppAccess(cfg, g.addrs())
	if err != nil {
		return err
	}
	raw, err := json.Marshal(norm)
	if err != nil {
		return err
	}
	if err := g.db.SetSettingRaw(ctx, appAccessSetting, raw); err != nil {
		return err
	}
	g.app.Store(&appAccessSnapshot{cfg: norm, rules: rules})
	return nil
}

// appProblems lists what is wrong with enabling the config for the caller: an empty white list refuses everybody, and the
// caller must not lose access.
func appProblems(ar *accessRules, caller netip.Addr) []accessProblem {
	out := []accessProblem{}
	if ar.mode == AccessOff {
		return out
	}
	if ar.mode == AccessWhite && ar.whiteN == 0 {
		out = append(out, accessProblem{Code: problemWhiteEmpty})
	}
	for _, h := range ar.unresolved {
		out = append(out, accessProblem{Code: problemHostNoAddr, Host: h})
	}
	if v := ar.decide(caller); !v.Allowed {
		out = append(out, accessProblem{Code: problemLockout, IP: v.IP, Reason: v.Reason})
	}
	return out
}

// resetApp turns the restriction of the application port off in the database (the lists stay); it reports a change.
func resetApp(ctx context.Context, g interface {
	SettingRaw(context.Context, string) []byte
	SetSettingRaw(context.Context, string, []byte) error
}) (bool, error) {
	raw := g.SettingRaw(ctx, appAccessSetting)
	if len(raw) == 0 {
		return false, nil
	}
	cfg := AppAccessConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		cfg = AppAccessConfig{}
	}
	changed := cfg.Mode != AccessOff && cfg.Mode != ""
	cfg.Mode = AccessOff
	norm, _, err := compileAppAccess(cfg, nil)
	if err != nil {
		return false, err
	}
	out, _ := json.Marshal(norm)
	return changed, g.SetSettingRaw(ctx, appAccessSetting, out)
}
