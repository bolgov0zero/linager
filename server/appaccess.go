package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
)

var errUnknownMode = errors.New("unknown mode")

// Access restriction of the application port: its own white or black list of addresses, apart from the restriction of the
// admin panel. The port is a direct TLS connection (nothing stands in front of it, the forwarding headers are dropped), so
// the lists judge the address of the connection itself; there are no proxies and no host names here.
//
// Until the list is saved for the first time, the application port follows the restriction of the admin panel (as it did
// before this setting existed), so an upgrade never opens a port that was closed.

const appAccessSetting = "app_access"

// AppAccessConfig is the restriction of the application port as the admin edits and stores it.
type AppAccessConfig struct {
	Mode  string       `json:"mode"`
	White []AccessRule `json:"white"`
	Black []AccessRule `json:"black"`
}

// appAccessSnapshot is the config in force; nil in the guard means "not saved yet: follow the admin panel".
type appAccessSnapshot struct {
	cfg   AppAccessConfig
	rules *accessRules
}

// compileAppAccess validates and normalises a config.
func compileAppAccess(cfg AppAccessConfig) (AppAccessConfig, *accessRules, error) {
	switch cfg.Mode {
	case AccessOff, AccessWhite, AccessBlack:
	default:
		return cfg, nil, errUnknownMode
	}
	ar := &accessRules{mode: cfg.Mode}
	var err error
	if cfg.White, ar.white, _, err = compileRules(cfg.White, maxAccessRules, false); err != nil {
		return cfg, nil, err
	}
	if cfg.Black, ar.black, _, err = compileRules(cfg.Black, maxAccessRules, false); err != nil {
		return cfg, nil, err
	}
	ar.whiteN, ar.blackN = len(cfg.White), len(cfg.Black)
	return cfg, ar, nil
}

// decideApp judges a connection to the application port.
func (g *AccessGuard) decideApp(peer netip.Addr) verdict {
	if s := g.app.Load(); s != nil {
		return s.rules.decide(peer, "")
	}
	return g.snap.Load().rules.decideDirect(peer)
}

// AppCurrent is the restriction of the application port in force and whether it was saved (false: it follows the admin panel).
func (g *AccessGuard) AppCurrent() (AppAccessConfig, bool) {
	if s := g.app.Load(); s != nil {
		return s.cfg, true
	}
	cfg := g.snap.Load().cfg
	return AppAccessConfig{Mode: cfg.Mode, White: cfg.White, Black: cfg.Black}, false
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
	norm, rules, err := compileAppAccess(cfg)
	if err != nil {
		g.log.Error("application access restriction: stored settings are invalid, the restriction is off", "err", err)
		norm, rules, _ = compileAppAccess(AppAccessConfig{Mode: AccessOff})
	}
	g.app.Store(&appAccessSnapshot{cfg: norm, rules: rules})
}

// SaveApp stores a validated config and puts it in force.
func (g *AccessGuard) SaveApp(ctx context.Context, cfg AppAccessConfig) error {
	norm, rules, err := compileAppAccess(cfg)
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
// caller (when the request came to the application port) must not lose access.
func appProblems(ar *accessRules, caller netip.Addr, direct bool) []accessProblem {
	out := []accessProblem{}
	if ar.mode == AccessOff {
		return out
	}
	if ar.mode == AccessWhite && ar.whiteN == 0 {
		out = append(out, accessProblem{Code: problemWhiteEmpty})
	}
	if direct {
		if v := ar.decide(caller, ""); !v.Allowed {
			out = append(out, accessProblem{Code: problemLockout, IP: v.IP, Reason: v.Reason})
		}
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
	norm, _, err := compileAppAccess(cfg)
	if err != nil {
		return false, err
	}
	out, _ := json.Marshal(norm)
	return changed, g.SetSettingRaw(ctx, appAccessSetting, out)
}
