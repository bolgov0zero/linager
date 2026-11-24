package server

import (
	"context"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/internal/winjoin"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/bolgov0zero/linager/server/store"
)

const (
	minJoinTTL = 5 * time.Minute
	maxJoinTTL = 30 * 24 * time.Hour
)

// joinOf returns the install link that an enrollment came with, when it is still valid for approval without a key.
func (a *Admin) joinOf(ctx context.Context, e *store.Enrollment) *store.JoinToken {
	if e.TokenID == 0 {
		return nil
	}
	t, err := a.DB.JoinTokenByID(ctx, e.TokenID)
	if err != nil || t.Revoked {
		return nil
	}
	return t
}

type joinView struct {
	ID          int64           `json:"id"`
	Label       string          `json:"label"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   int64           `json:"created_at"`
	ExpiresAt   int64           `json:"expires_at"`
	MaxUses     int             `json:"max_uses"`
	Uses        int             `json:"uses"`
	AutoApprove bool            `json:"auto_approve"`
	Networks    []string        `json:"networks"`
	GroupID     *int64          `json:"group_id"`
	GroupName   string          `json:"group_name,omitempty"`
	Used        []store.JoinUse `json:"used"`
}

func (a *Admin) joinViewOf(ctx context.Context, t *store.JoinToken) joinView {
	v := joinView{ID: t.ID, Label: t.Label, CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt.Unix(), ExpiresAt: t.ExpiresAt.Unix(), MaxUses: t.MaxUses,
		Uses: t.Uses, AutoApprove: t.AutoApprove, Networks: t.Networks, GroupID: t.GroupID, Used: t.Used}
	if v.Networks == nil {
		v.Networks = []string{}
	}
	if v.Used == nil {
		v.Used = []store.JoinUse{}
	}
	if t.GroupID != nil {
		v.GroupName, _ = a.DB.GroupPath(ctx, *t.GroupID)
	}
	return v
}

func (a *Admin) joinList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := a.DB.JoinTokens(r.Context())
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]joinView, 0, len(list))
	for _, t := range list {
		out = append(out, a.joinViewOf(r.Context(), t))
	}
	writeJSON(w, http.StatusOK, out)
}

type joinCommand struct {
	Kind     string `json:"kind"`     // external or local
	Platform string `json:"platform"` // linux or windows
	Host     string `json:"host"`
	Command  string `json:"command"`
}

type joinCreated struct {
	joinView
	// Token is shown once; the panel keeps its hash only.
	Token    string        `json:"token"`
	Commands []joinCommand `json:"commands"`
}

// installCommand is the line a person runs on the new host, as root. It uses wget (the base system of Debian has
// it, curl is an extra): -nv keeps the errors visible, because an empty script piped into sh would end silently.
func installCommand(host, port, pin, secret string) string {
	return "wget -nv -O- --no-check-certificate --pinnedpubkey " + shq(pin) + " " + shq("https://"+net.JoinHostPort(host, port)+"/v1/join/"+secret) + " | sh"
}

func (a *Admin) joinCreate(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		TTLSeconds  int      `json:"ttl_seconds"`
		MaxUses     int      `json:"max_uses"`
		AutoApprove bool     `json:"auto_approve"`
		Networks    []string `json:"networks"`
		GroupID     *int64   `json:"group_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl < minJoinTTL || ttl > maxJoinTTL || req.MaxUses < 0 || req.MaxUses > 1000 {
		jsonErr(w, http.StatusUnprocessableEntity, "invalid lifetime or number of servers")
		return
	}
	nets, err := parseNetworks(req.Networks)
	if err != nil {
		jsonErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if req.GroupID != nil {
		if _, err := a.DB.GroupName(r.Context(), *req.GroupID); err != nil {
			jsonErr(w, http.StatusUnprocessableEntity, "unknown group")
			return
		}
	}
	port := a.agentPort()
	pin, err := spkiPin(a.Keys.TLSDER)
	if err != nil || port == "" {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	ext, loc := joinHosts(a.Cfg, r.Host)
	if ext == "" && loc == "" {
		jsonErr(w, http.StatusUnprocessableEntity, "no address of this server is usable from another host; list it in agents.hosts")
		return
	}
	secret, label, hash, err := newJoinSecret()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	t, err := a.DB.CreateJoinToken(r.Context(), store.JoinToken{Label: label, Kind: "link", CreatedBy: u.Login, ExpiresAt: time.Now().Add(ttl),
		MaxUses: req.MaxUses, AutoApprove: req.AutoApprove, Networks: nets, GroupID: req.GroupID}, hash)
	if err != nil {
		a.Log.Error("create a join token", "err", err)
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := joinCreated{joinView: a.joinViewOf(r.Context(), t), Token: secret}
	fp := pki.Fingerprint(a.Keys.TLSAltDER) // the pin of the Windows command: the certificate a Windows client is given
	if ext != "" {
		out.Commands = append(out.Commands, joinCommand{Kind: "external", Platform: "linux", Host: ext, Command: installCommand(ext, port, pin, secret)},
			joinCommand{Kind: "external", Platform: "windows", Host: ext, Command: winjoin.InstallCommand(ext, port, fp, secret)})
	}
	if loc != "" {
		out.Commands = append(out.Commands, joinCommand{Kind: "local", Platform: "linux", Host: loc, Command: installCommand(loc, port, pin, secret)},
			joinCommand{Kind: "local", Platform: "windows", Host: loc, Command: winjoin.InstallCommand(loc, port, fp, secret)})
	}
	// the first host that uses the link should not wait for the release to be downloaded
	if a.Updates != nil {
		go func() { //nolint:gosec // a download that outlives the request on purpose
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			_, _ = a.Updates.CurrentRelease(ctx)
		}()
	}
	a.audit(r, u, "join.create", "join_token", strconv.FormatInt(t.ID, 10), label, "ok", map[string]any{
		"ttl_seconds": req.TTLSeconds, "max_uses": req.MaxUses, "auto_approve": req.AutoApprove, "networks": nets})
	writeJSON(w, http.StatusOK, out)
}

func (a *Admin) joinRevoke(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, http.StatusNotFound, "link not found")
		return
	}
	t, err := a.DB.JoinTokenByID(r.Context(), id)
	if err != nil || t.Kind != "link" {
		jsonErr(w, http.StatusNotFound, "link not found")
		return
	}
	if err := a.DB.RevokeJoinToken(r.Context(), id); err != nil {
		jsonErr(w, http.StatusNotFound, "link not found")
		return
	}
	a.audit(r, u, "join.revoke", "join_token", strconv.FormatInt(id, 10), t.Label, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
