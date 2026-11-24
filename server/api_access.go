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

// Settings, "Access to the admin panel": the restriction, the trusted proxies and the journal of visits.

type accessYou struct {
	IP           string `json:"ip"`            // the address the restriction judges
	RemoteAddr   string `json:"remote_addr"`   // the address of the connection (the proxy, or the client itself)
	ForwardedFor string `json:"forwarded_for"` // X-Forwarded-For as received
	ViaProxy     bool   `json:"via_proxy"`     // the request carries X-Forwarded-For
	PeerTrusted  bool   `json:"peer_trusted"`  // the connection is made by a trusted proxy
}

type accessState struct {
	Config     AccessConfig `json:"config"`
	Configured bool         `json:"configured"` // false: the proxies are still the ones of server.yaml
	You        accessYou    `json:"you"`
	Denied24h  int          `json:"denied_24h"`
	// Hosts describes the host names of the lists (what each points to now).
	Hosts map[string]HostStatus `json:"hosts"`
}

func (a *Admin) accessYou(r *http.Request, ar *accessRules) accessYou {
	peer, xff := remoteAddr(r), r.Header.Get("X-Forwarded-For")
	v := ar.decide(peer, xff)
	return accessYou{IP: v.IP, RemoteAddr: peer.String(), ForwardedFor: xff, ViaProxy: xff != "", PeerTrusted: v.ViaProxy}
}

func (a *Admin) accessState(r *http.Request) accessState {
	snap := a.Access.snap.Load()
	n, _ := a.DB.AccessDeniedSince(r.Context(), time.Now().Add(-24*time.Hour), false)
	return accessState{Config: snap.cfg, Configured: snap.configured, You: a.accessYou(r, snap.rules), Denied24h: n, Hosts: a.Access.hostStatuses(snap.rules.hosts)}
}

func (a *Admin) accessGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, a.accessState(r))
}

type accessCheckReq struct {
	Config AccessConfig `json:"config"`
	Test   *struct {
		IP    string `json:"ip"`
		Proxy bool   `json:"proxy"` // the test request comes through a trusted proxy
	} `json:"test"`
}

type accessCheckResp struct {
	Problems []accessProblem       `json:"problems"`
	You      verdict               `json:"you"`
	Test     *verdict              `json:"test,omitempty"`
	Hosts    map[string]HostStatus `json:"hosts"`
}

// check judges a draft config for the request that sends it: what would happen to the caller, whether it may be
// applied, and optionally what would happen to a given address.
func (a *Admin) check(r *http.Request, cfg AccessConfig, test *struct {
	IP    string `json:"ip"`
	Proxy bool   `json:"proxy"`
}) (AccessConfig, accessCheckResp, error) {
	_, first, err := compileAccess(cfg, nil)
	if err != nil {
		return cfg, accessCheckResp{}, err
	}
	// the host names of the draft are looked up (a name that was asked about a moment ago is not asked again)
	a.Access.lookupHosts(r.Context(), first.hosts, hostCheckMaxAge)
	norm, ar, err := compileAccess(cfg, a.Access.addrs())
	if err != nil {
		return cfg, accessCheckResp{}, err
	}
	peer, xff := remoteAddr(r), r.Header.Get("X-Forwarded-For")
	resp := accessCheckResp{Problems: ar.problems(peer, xff), You: ar.decide(peer, xff), Hosts: a.Access.hostStatuses(ar.hosts)}
	if test != nil {
		if ip, err := netip.ParseAddr(strings.TrimSpace(test.IP)); err == nil {
			ip = ip.Unmap()
			var v verdict
			switch {
			case !test.Proxy:
				v = ar.decide(ip, "")
			case len(ar.proxies) > 0 && !ar.trustAllProxies:
				v = ar.decide(ar.proxies[0].Addr(), ip.String())
			case ar.trustAllProxies:
				v = ar.decide(peer, ip.String())
			default: // no trusted proxy: there is nobody to carry the address, it would be judged as the proxy itself
				v = ar.decide(ip, "")
				v.IgnoredXFF = true
			}
			resp.Test = &v
		}
	}
	return norm, resp, nil
}

func (a *Admin) accessCheck(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var req accessCheckReq
	if !decode(w, r, &req) {
		return
	}
	_, resp, err := a.check(r, req.Config, req.Test)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_config", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Admin) accessPut(w http.ResponseWriter, r *http.Request, u *store.User) {
	var cfg AccessConfig
	if !decode(w, r, &cfg) {
		return
	}
	norm, resp, err := a.check(r, cfg, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_config", "detail": err.Error()})
		return
	}
	if len(resp.Problems) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "access_problems", "problems": resp.Problems})
		return
	}
	if err := a.Access.Save(r.Context(), norm); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "access.update", "access", "", norm.Mode, "ok", map[string]any{"mode": norm.Mode, "only_proxy": norm.OnlyProxy,
		"white": len(norm.White), "black": len(norm.Black), "proxies": len(norm.Proxies)})
	writeJSON(w, http.StatusOK, a.accessState(r))
}

func (a *Admin) accessRetention(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Days int `json:"days"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Days < 1 || req.Days > 365 {
		jsonErr(w, http.StatusBadRequest, "bad retention")
		return
	}
	cfg, _ := a.Access.Current()
	cfg.Retention = req.Days
	if err := a.Access.Save(r.Context(), cfg); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "access.retention", "access", "", strconv.Itoa(req.Days), "ok", map[string]any{"days": req.Days})
	writeJSON(w, http.StatusOK, a.accessState(r))
}

func (a *Admin) accessLog(w http.ResponseWriter, r *http.Request, _ *store.User) {
	q := r.URL.Query()
	filter := q.Get("filter")
	switch filter {
	case "all", "denied", "login", "fail", "proxy", "direct":
	case "":
		filter = "all"
	default:
		jsonErr(w, http.StatusBadRequest, "bad filter")
		return
	}
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, err := a.DB.AccessLogList(r.Context(), filter, before, limit, q.Get("port") == "app")
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// accessResolve looks a host name up right now (the interface asks when a name is added and by the button that
// refreshes it). A name that is already in a list gets its new address in force at once.
func (a *Admin) accessResolve(w http.ResponseWriter, r *http.Request, _ *store.User) {
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
	tracked := slices.Contains(g.snap.Load().rules.hosts, h)
	if tracked {
		g.reapply()
		g.persistHosts(r.Context(), g.snap.Load().rules.hosts)
	}
	st := g.hostStatuses([]string{h})[h]
	if len(st.IPs) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "unresolved", "host": h})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": h, "status": st})
}
