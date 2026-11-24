package server

import (
	"context"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/internal/release"
	"github.com/bolgov0zero/linager/server/store"
)

var sshUserRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,31}$`)

// newSSHInstaller wires the installer to the rest of the server.
func (a *Admin) newSSHInstaller() *SSHInstaller {
	_, port, _ := net.SplitHostPort(a.Cfg.Agents.Listen)
	var pin, fp string
	if a.Keys != nil { // some tests build an Admin without keys
		pin, _ = spkiPin(a.Keys.TLSDER)
		fp = pkiFingerprint(a.Keys.TLSDER)
	}
	deps := SSHDeps{
		Port:        port,
		Fingerprint: fp,
		Pin:         pin,
		Hosts: func() []string {
			ext, loc := joinHosts(a.Cfg, "")
			out := []string{loc, ext}
			out = append(out, a.Cfg.Agents.Hosts...)
			return out
		},
		Assets: func(ctx context.Context) (map[string]release.Asset, error) {
			c, err := a.Updates.CurrentRelease(ctx)
			if err != nil {
				return nil, err
			}
			return c.Manifest.AssetList(), nil
		},
		NewToken: func(ctx context.Context, by string, group *int64) (string, int64, error) {
			secret, label, hash, err := newJoinSecret()
			if err != nil {
				return "", 0, err
			}
			t, err := a.DB.CreateJoinToken(ctx, store.JoinToken{Label: label, Kind: "ssh", CreatedBy: by, ExpiresAt: time.Now().Add(sshTokenTTL),
				MaxUses: 1, AutoApprove: true, GroupID: group}, hash)
			if err != nil {
				return "", 0, err
			}
			return secret, t.ID, nil
		},
		EndToken: func(ctx context.Context, id int64) { _ = a.DB.EndJoinToken(ctx, id) },
		Observe: func(ctx context.Context, tokenID int64) (string, string, bool, bool) {
			e, err := a.DB.EnrollmentByToken(ctx, tokenID)
			if err != nil || e.ServerID == "" {
				return "", "", false, false
			}
			srv, err := a.DB.ServerByID(ctx, e.ServerID)
			if err != nil {
				return "", "", false, false
			}
			return srv.ID, srv.Name, true, a.Hub.Online(srv.ID)
		},
		Forget: func(ctx context.Context, serverID string) {
			a.Hub.Kick(serverID)
			_ = a.DB.DeleteServer(ctx, serverID)
		},
		Audit: func(ctx context.Context, user, action, target, result string, details map[string]any) {
			_ = a.DB.WriteAudit(ctx, store.Audit{Actor: user, Action: action, TargetType: "host", TargetName: target, Result: result, Details: details})
		},
	}
	return NewSSHInstaller(deps, a.Cfg.SSHInstall.AllowedNetworks)
}

func pkiFingerprint(der []byte) string { return fingerprintOf(der) }

type sshStartReq struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Login      string `json:"login"`
	Auth       string `json:"auth"`
	Password   string `json:"password"`
	Key        string `json:"key"`
	Passphrase string `json:"passphrase"`
	GroupID    *int64 `json:"group_id"`
}

func (a *Admin) sshStart(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req sshStartReq
	if !decode(w, r, &req) {
		return
	}
	req.Host = strings.TrimSpace(req.Host)
	if req.Port == 0 {
		req.Port = 22
	}
	secret := req.Password
	if req.Auth == "key" {
		secret = req.Key
	}
	switch {
	case req.Host == "" || len(req.Host) > 253 || !addrRe.MatchString(req.Host),
		req.Port < 1 || req.Port > 65535,
		!sshUserRe.MatchString(req.Login),
		req.Auth != "password" && req.Auth != "key",
		secret == "" || len(secret) > 64<<10:
		jsonErr(w, http.StatusUnprocessableEntity, "invalid host, port, login or secret")
		return
	}
	// a password must not cross the network in the clear
	if r.TLS == nil {
		jsonErr(w, http.StatusUnprocessableEntity, "connect over TLS to enter credentials")
		return
	}
	if req.GroupID != nil {
		if _, err := a.DB.GroupName(r.Context(), *req.GroupID); err != nil {
			jsonErr(w, http.StatusUnprocessableEntity, "unknown group")
			return
		}
	}
	if !a.sshL.Allow(u.Login) {
		jsonErr(w, http.StatusTooManyRequests, "too many attempts, wait a few minutes")
		return
	}
	s, err := a.SSH.Start(u.ID, u.Login, SSHStart{Host: req.Host, Port: req.Port, Login: req.Login, Auth: req.Auth, Password: req.Password,
		Key: req.Key, Passphrase: req.Passphrase, GroupID: req.GroupID, PanelHost: hostOnly(r.Host)})
	if err != nil {
		code := http.StatusUnprocessableEntity
		jsonErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": s.id})
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

func (a *Admin) sshSession(w http.ResponseWriter, r *http.Request, u *store.User) (*sshSession, bool) {
	s, ok := a.SSH.Get(r.PathValue("id"), u.ID)
	if !ok {
		jsonErr(w, http.StatusNotFound, "install not found or expired")
		return nil, false
	}
	return s, true
}

func (a *Admin) sshGet(w http.ResponseWriter, r *http.Request, u *store.User) {
	if s, ok := a.sshSession(w, r, u); ok {
		writeJSON(w, http.StatusOK, s.State())
	}
}

func (a *Admin) sshAnswer(w http.ResponseWriter, r *http.Request, u *store.User) {
	s, ok := a.sshSession(w, r, u)
	if !ok {
		return
	}
	var req struct {
		Gate     string `json:"gate"`
		Accept   bool   `json:"accept"`
		Password string `json:"password"`
		Address  string `json:"address"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.Answer(req.Gate, gateAnswer{Accept: req.Accept, Password: req.Password, Address: req.Address}); err != nil {
		jsonErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.State())
}

func (a *Admin) sshInstall(w http.ResponseWriter, r *http.Request, u *store.User) {
	s, ok := a.sshSession(w, r, u)
	if !ok {
		return
	}
	if err := s.StartInstall(); err != nil {
		jsonErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.State())
}

func (a *Admin) sshClose(w http.ResponseWriter, r *http.Request, u *store.User) {
	a.SSH.Close(r.PathValue("id"), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// RunJoinPurge removes old tokens once an hour.
func (a *Admin) RunJoinPurge(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		_ = a.DB.PurgeJoinTokens(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
