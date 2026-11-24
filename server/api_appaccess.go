package server

import (
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/server/store"
)

// Settings, "Server access": the white or black list of the application port, a test of an address and the journal of the
// connections.

type appAccessState struct {
	Config    AppAccessConfig `json:"config"`
	You       *verdict        `json:"you,omitempty"`
	Denied24h int             `json:"denied_24h"`
	// Hosts describes the host names of the lists (what each points to now).
	Hosts map[string]HostStatus `json:"hosts"`
}

func (a *Admin) appAccessState(r *http.Request) appAccessState {
	cfg := a.Access.AppCurrent()
	n, _ := a.DB.AccessDeniedSince(r.Context(), time.Now().Add(-24*time.Hour))
	v := a.Access.decideApp(remoteAddr(r))
	return appAccessState{Config: cfg, You: &v, Denied24h: n, Hosts: a.Access.hostStatuses(appHostNames(cfg))}
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
	caller := remoteAddr(r).Unmap()
	you := ar.decide(caller)
	resp := appAccessCheckResp{Problems: appProblems(ar, caller), You: &you, Hosts: a.Access.hostStatuses(ar.hosts)}
	if ip, err := netip.ParseAddr(strings.TrimSpace(testIP)); err == nil {
		v := ar.decide(ip.Unmap())
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

func (a *Admin) appAccessLog(w http.ResponseWriter, r *http.Request, _ *store.User) {
	q := r.URL.Query()
	filter := q.Get("filter")
	switch filter {
	case "all", "denied", "login", "fail":
	case "":
		filter = "all"
	default:
		jsonErr(w, http.StatusBadRequest, "bad filter")
		return
	}
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, err := a.DB.AccessLogList(r.Context(), filter, before, limit)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// appAccessResolve looks a host name up right now (the interface asks when a name is added and by the button that
// refreshes it). A name that is already in the list gets its new address in force at once.
func (a *Admin) appAccessResolve(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var req struct {
		Host string `json:"host"`
	}
	if !decode(w, r, &req) {
		return
	}
	h, ok := normalizeHost(req.Host)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_host"})
		return
	}
	g := a.Access
	g.lookupHosts(r.Context(), []string{h}, 0)
	if slices.Contains(g.trackedHosts(), h) {
		g.reapplyApp()
		g.persistHosts(r.Context(), g.trackedHosts())
	}
	st := g.hostStatuses([]string{h})[h]
	if len(st.IPs) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "unresolved", "host": h, "status": st})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": h, "status": st})
}
