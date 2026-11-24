package server

import (
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/server/store"
)

// Settings, "Access of the application": the white or black list of the application port, apart from the admin panel.

type appAccessState struct {
	Config     AppAccessConfig `json:"config"`
	Configured bool            `json:"configured"` // false: the port still follows the restriction of the admin panel
	You        *verdict        `json:"you,omitempty"`
	Denied24h  int             `json:"denied_24h"`
	// Hosts describes the host names of the lists (what each points to now).
	Hosts map[string]HostStatus `json:"hosts"`
}

// appCaller is the address the application port would judge for this request: the address of the connection when it came
// to the application port; for a request that came through the admin panel there is no such address (direct is false).
func appCaller(r *http.Request) (netip.Addr, bool) {
	if !isDirect(r) {
		return netip.Addr{}, false
	}
	return remoteAddr(r).Unmap(), true
}

func (a *Admin) appAccessState(r *http.Request) appAccessState {
	cfg, configured := a.Access.AppCurrent()
	n, _ := a.DB.AccessDeniedSince(r.Context(), time.Now().Add(-24*time.Hour), true)
	st := appAccessState{Config: cfg, Configured: configured, Denied24h: n, Hosts: a.Access.hostStatuses(appHostNames(cfg))}
	if ip, direct := appCaller(r); direct {
		v := a.Access.decideApp(ip)
		st.You = &v
	}
	return st
}

func (a *Admin) appAccessGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, a.appAccessState(r))
}

type appAccessCheckResp struct {
	Problems []accessProblem       `json:"problems"`
	You      *verdict              `json:"you,omitempty"`
	Test     *verdict              `json:"test,omitempty"`
	Hosts    map[string]HostStatus `json:"hosts"`
}

type appAccessCheckReq struct {
	Config AppAccessConfig `json:"config"`
	TestIP string          `json:"test_ip"`
}

func (a *Admin) appCheck(r *http.Request, cfg AppAccessConfig, testIP string) (AppAccessConfig, appAccessCheckResp, error) {
	_, first, err := compileAppAccess(cfg, nil)
	if err != nil {
		return cfg, appAccessCheckResp{}, err
	}
	// the host names of the draft are looked up (a name that was asked about a moment ago is not asked again)
	a.Access.lookupHosts(r.Context(), first.hosts, hostCheckMaxAge)
	norm, ar, err := compileAppAccess(cfg, a.Access.addrs())
	if err != nil {
		return cfg, appAccessCheckResp{}, err
	}
	caller, direct := appCaller(r)
	resp := appAccessCheckResp{Problems: appProblems(ar, caller, direct), Hosts: a.Access.hostStatuses(ar.hosts)}
	if direct {
		v := ar.decide(caller, "")
		resp.You = &v
	}
	if ip, err := netip.ParseAddr(strings.TrimSpace(testIP)); err == nil {
		v := ar.decide(ip.Unmap(), "")
		resp.Test = &v
	}
	return norm, resp, nil
}

func (a *Admin) appAccessCheck(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var req appAccessCheckReq
	if !decode(w, r, &req) {
		return
	}
	_, resp, err := a.appCheck(r, req.Config, req.TestIP)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_config", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Admin) appAccessPut(w http.ResponseWriter, r *http.Request, u *store.User) {
	var cfg AppAccessConfig
	if !decode(w, r, &cfg) {
		return
	}
	norm, resp, err := a.appCheck(r, cfg, "")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_config", "detail": err.Error()})
		return
	}
	if len(resp.Problems) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "access_problems", "problems": resp.Problems})
		return
	}
	if err := a.Access.SaveApp(r.Context(), norm); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.Access.persistHosts(r.Context(), a.Access.trackedHosts())
	a.audit(r, u, "appaccess.update", "access", "", norm.Mode, "ok", map[string]any{"mode": norm.Mode, "white": len(norm.White), "black": len(norm.Black)})
	writeJSON(w, http.StatusOK, a.appAccessState(r))
}
