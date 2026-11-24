package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/bolgov0zero/linager/agent"
	"github.com/bolgov0zero/linager/internal/protocol"
	"github.com/bolgov0zero/linager/server/store"
)

// callErr maps agent-call errors to HTTP responses.
func callErr(w http.ResponseWriter, err error) {
	var ae *AgentError
	switch {
	case errors.Is(err, ErrOffline), errors.Is(err, ErrAgentDisconect):
		jsonErr(w, http.StatusServiceUnavailable, "agent is offline")
	case errors.Is(err, ErrCallTimeout):
		jsonErr(w, http.StatusGatewayTimeout, "agent did not answer in time")
	case errors.As(err, &ae) && ae.Msg == protocol.ErrModuleDisabled:
		jsonErr(w, http.StatusNotImplemented, protocol.ErrModuleDisabled)
	case errors.As(err, &ae) && ae.Msg == protocol.ErrBusy:
		jsonErr(w, http.StatusConflict, protocol.ErrBusy)
	case errors.As(err, &ae) && ae.Msg == protocol.ErrTooLarge:
		jsonErr(w, http.StatusRequestEntityTooLarge, ae.Msg)
	case errors.As(err, &ae) && ae.Msg == protocol.ErrBinary:
		jsonErr(w, http.StatusUnsupportedMediaType, ae.Msg)
	case errors.As(err, &ae) && (ae.Msg == protocol.ErrConflict || ae.Msg == protocol.ErrExists):
		jsonErr(w, http.StatusConflict, ae.Msg)
	case errors.As(err, &ae) && ae.Msg == protocol.ErrNotFound:
		jsonErr(w, http.StatusNotFound, ae.Msg)
	case errors.As(err, &ae) && ae.Msg == "permission denied":
		jsonErr(w, http.StatusForbidden, ae.Msg)
	case errors.As(err, &ae) && ae.Msg == protocol.ErrNoPending:
		jsonErr(w, http.StatusGone, ae.Msg)
	case errors.As(err, &ae) && dockerConflict(ae.Msg):
		jsonErr(w, http.StatusConflict, ae.Msg)
	case errors.As(err, &ae):
		jsonErr(w, http.StatusBadGateway, ae.Msg)
	default:
		jsonErr(w, http.StatusInternalServerError, "internal error")
	}
}

func dockerConflict(m string) bool {
	m = strings.ToLower(m)
	return strings.Contains(m, "conflict") || strings.Contains(m, "being used") || strings.Contains(m, "in use")
}

func (a *Admin) serverOr404(w http.ResponseWriter, r *http.Request) *store.Server {
	s, err := a.DB.ServerByID(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, http.StatusNotFound, "server not found")
		return nil
	}
	return s
}

// proxyCall runs a read-only command and writes the agent's JSON answer through.
func (a *Admin) proxyCall(w http.ResponseWriter, r *http.Request, module, action string, params any, timeout time.Duration) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	data, err := a.Hub.Call(r.Context(), s.ID, module, action, params, false, timeout)
	if err != nil {
		callErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (a *Admin) servicesList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	a.proxyCall(w, r, "services", "list", nil, 30*time.Second)
}

// servicesIgnored returns the units whose failure does not count towards the server status.
func (a *Admin) servicesIgnored(w http.ResponseWriter, r *http.Request, _ *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	list, err := a.DB.IgnoredServices(r.Context(), s.ID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ignored": list})
}

// servicesIgnore adds a unit to the ignore list or removes it.
func (a *Admin) servicesIgnore(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req struct {
		Unit   string `json:"unit"`
		Ignore bool   `json:"ignore"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Unit == "" || len(req.Unit) > 256 || strings.ContainsAny(req.Unit, " \t\n/") {
		jsonErr(w, http.StatusBadRequest, "invalid unit")
		return
	}
	if err := a.DB.SetServiceIgnored(r.Context(), s.ID, req.Unit, req.Ignore); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	list, _ := a.DB.IgnoredServices(r.Context(), s.ID)
	a.Metrics.SetIgnored(r.Context(), s.ID, list)
	action := "service.ignore"
	if !req.Ignore {
		action = "service.unignore"
	}
	a.audit(r, u, action, "server", s.ID, s.Name, "ok", map[string]any{"unit": req.Unit})
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ignoreUpdates adds a package (module "apt") or a container (module "docker") to the list the agent keeps of the
// ignored ones, or removes it: the item stays in the lists of the panel but is not counted in the badges.
func (a *Admin) ignoreUpdates(module string) func(w http.ResponseWriter, r *http.Request, u *store.User) {
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		s := a.serverOr404(w, r)
		if s == nil {
			return
		}
		var req struct {
			Name   string `json:"name"`
			Ignore bool   `json:"ignore"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Name == "" || len(req.Name) > 128 || strings.ContainsAny(req.Name, " \t\n/") {
			jsonErr(w, http.StatusBadRequest, "invalid name")
			return
		}
		_, err := a.Hub.Call(r.Context(), s.ID, module, "ignore", map[string]any{"name": req.Name, "ignore": req.Ignore}, false, 30*time.Second)
		action := module + ".ignore"
		if !req.Ignore {
			action = module + ".unignore"
		}
		res := "ok"
		if err != nil {
			res = "failed"
		}
		a.audit(r, u, action, "server", s.ID, s.Name, res, map[string]any{"name": req.Name})
		if err != nil {
			callErr(w, err)
			return
		}
		a.Live.Publish("dashboard", map[string]any{})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (a *Admin) aptList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	a.proxyCall(w, r, "apt", "list", nil, 90*time.Second)
}

func (a *Admin) processesList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	sortBy := r.URL.Query().Get("sort")
	if sortBy != "mem" && sortBy != "name" {
		sortBy = "cpu"
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	a.proxyCall(w, r, "processes", "list", map[string]any{"sort": sortBy, "n": n}, 20*time.Second)
}

// securityState is the read-only SSH overview: logins, failed attempts, fail2ban, keys. The agent collects
// it from the journal and answers from a one minute cache.
func (a *Admin) securityState(w http.ResponseWriter, r *http.Request, _ *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	data, err := a.Hub.Call(r.Context(), s.ID, "security", "state", nil, false, 40*time.Second)
	if err != nil {
		callErr(w, err)
		return
	}
	// the address the administrator comes from: the panel never offers to block it
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) == nil {
		self, _ := json.Marshal(clientIP(r))
		obj["self_ip"] = self
		if merged, err := json.Marshal(obj); err == nil {
			data = merged
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

var (
	termSessionRe = regexp.MustCompile(`^[a-zA-Z0-9]{1,16}$`)
	termUserRe    = regexp.MustCompile(`^[a-zA-Z0-9._][a-zA-Z0-9._-]{0,63}$`)
)

type terminateReq struct {
	Session string `json:"session"`
	User    string `json:"user"`
	Confirm bool   `json:"confirm"`
}

// securityTerminate ends remote (SSH) sessions: one by id or all of a user. The processes of the session
// stop with it, so it needs the confirmation of the administrator; the agent only ends sessions from its own
// list of remote sessions. The result of every ended session goes to the audit log.
func (a *Admin) securityTerminate(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req terminateReq
	if !decode(w, r, &req) {
		return
	}
	if (req.Session == "") == (req.User == "") ||
		(req.Session != "" && !termSessionRe.MatchString(req.Session)) ||
		(req.User != "" && !termUserRe.MatchString(req.User)) {
		jsonErr(w, http.StatusBadRequest, "give either a session or a user")
		return
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	params := map[string]string{}
	if req.Session != "" {
		params["session"] = req.Session
	} else {
		params["user"] = req.User
	}
	data, err := a.Hub.Call(r.Context(), s.ID, "security", "terminate", params, true, 30*time.Second)
	details := map[string]any{"session": req.Session, "user": req.User}
	if err == nil {
		var res struct {
			Terminated []map[string]any `json:"terminated"`
		}
		if json.Unmarshal(data, &res) == nil {
			details["terminated"] = res.Terminated
		}
	}
	result := "ok"
	if err != nil {
		result = "failed"
	}
	a.audit(r, u, "security.session.terminate", "server", s.ID, s.Name, result, details)
	if err != nil {
		callErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// secCall sends a confirmed security command to the agent and writes the answer, the audit entry included.
func (a *Admin) secCall(w http.ResponseWriter, r *http.Request, u *store.User, s *store.Server, module, cmd, action string, params map[string]any, details map[string]any, timeout time.Duration) {
	data, err := a.Hub.Call(r.Context(), s.ID, module, cmd, params, true, timeout)
	result := "ok"
	if err != nil {
		result = "failed"
	}
	a.audit(r, u, action, "server", s.ID, s.Name, result, details)
	if err != nil {
		callErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data) //nolint:gosec // the JSON answer of our own agent
}

type f2bBanReq struct {
	IP       string `json:"ip"`
	Duration int    `json:"duration"`
	Confirm  bool   `json:"confirm"`
}

// securityBan blocks an address through fail2ban. The address of the administrator is sent along, so the
// agent can refuse to block it.
func (a *Admin) securityBan(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req f2bBanReq
	if !decode(w, r, &req) {
		return
	}
	if net.ParseIP(req.IP) == nil {
		jsonErr(w, http.StatusBadRequest, "invalid address")
		return
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	a.secCall(w, r, u, s, "security", "ban", "security.fail2ban.ban", map[string]any{"ip": req.IP, "duration": req.Duration, "self": clientIP(r)},
		map[string]any{"ip": req.IP, "duration": req.Duration}, 40*time.Second)
}

type f2bUnbanReq struct {
	IPs     []string `json:"ips"`
	All     bool     `json:"all"`
	Confirm bool     `json:"confirm"`
}

func (a *Admin) securityUnban(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req f2bUnbanReq
	if !decode(w, r, &req) {
		return
	}
	if len(req.IPs) > 500 || (!req.All && len(req.IPs) == 0) {
		jsonErr(w, http.StatusBadRequest, "give the addresses to unblock")
		return
	}
	for _, ip := range req.IPs {
		if net.ParseIP(ip) == nil {
			jsonErr(w, http.StatusBadRequest, "invalid address")
			return
		}
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	details := map[string]any{"all": req.All, "count": len(req.IPs)}
	if len(req.IPs) <= 20 {
		details["ips"] = req.IPs
	}
	a.secCall(w, r, u, s, "security", "unban", "security.fail2ban.unban", map[string]any{"ips": req.IPs, "all": req.All}, details, 90*time.Second)
}

type f2bSettingsReq struct {
	MaxRetry  int      `json:"maxretry"`
	FindTime  int      `json:"findtime"`
	BanTime   int      `json:"bantime"`
	Increment bool     `json:"increment"`
	Recidive  bool     `json:"recidive"`
	Ignore    []string `json:"ignore"`
	Confirm   bool     `json:"confirm"`
}

// securityF2bSettings saves the parameters of the sshd jail; the agent writes its own file and reloads fail2ban.
func (a *Admin) securityF2bSettings(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req f2bSettingsReq
	if !decode(w, r, &req) {
		return
	}
	if len(req.Ignore) > 200 {
		jsonErr(w, http.StatusBadRequest, "too many addresses")
		return
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	params := map[string]any{"maxretry": req.MaxRetry, "findtime": req.FindTime, "bantime": req.BanTime, "increment": req.Increment,
		"recidive": req.Recidive, "ignore": req.Ignore, "self": clientIP(r)}
	a.secCall(w, r, u, s, "security", "f2bSettings", "security.fail2ban.settings", params,
		map[string]any{"maxretry": req.MaxRetry, "findtime": req.FindTime, "bantime": req.BanTime, "increment": req.Increment, "recidive": req.Recidive, "ignore": len(req.Ignore)}, 90*time.Second)
}

// securityBanHistory reads the ban and unban events from the log of fail2ban.
func (a *Admin) securityBanHistory(w http.ResponseWriter, r *http.Request, _ *store.User) {
	a.proxyCall(w, r, "security", "banHistory", map[string]any{"limit": atoiClamp(r.URL.Query().Get("limit"), 1, 1000, 200)}, 30*time.Second)
}

type keyRemoveReq struct {
	User        string `json:"user"`
	File        string `json:"file"`
	Fingerprint string `json:"fingerprint"`
	Confirm     bool   `json:"confirm"`
}

var fingerprintRe = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)

// securityKeyRemove deletes a key from authorized_keys; the agent only accepts the files it lists itself.
func (a *Admin) securityKeyRemove(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req keyRemoveReq
	if !decode(w, r, &req) {
		return
	}
	if !termUserRe.MatchString(req.User) || !fingerprintRe.MatchString(req.Fingerprint) || req.File == "" || len(req.File) > 512 {
		jsonErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	a.secCall(w, r, u, s, "security", "keyRemove", "security.key.remove", map[string]any{"user": req.User, "file": req.File, "fingerprint": req.Fingerprint},
		map[string]any{"user": req.User, "file": req.File, "fingerprint": req.Fingerprint}, 30*time.Second)
}

// ---- disk ----

func (a *Admin) diskState(w http.ResponseWriter, r *http.Request, _ *store.User) {
	a.proxyCall(w, r, "disk", "state", nil, 40*time.Second)
}

// validMount accepts a mount point of Linux ("/", "/var") and a volume of Windows ("C:"). The agent checks that the
// volume is one it lists itself.
func validMount(m string) bool {
	if m == "" || len(m) > 512 {
		return false
	}
	if strings.HasPrefix(m, "/") {
		return true
	}
	return len(m) >= 2 && len(m) <= 3 && m[1] == ':' && (m[0]|0x20) >= 'a' && (m[0]|0x20) <= 'z' && (len(m) == 2 || m[2] == '\\' || m[2] == '/')
}

// diskScan measures what takes the space on one filesystem; it can take a minute on a big one.
func (a *Admin) diskScan(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req struct {
		Mount string `json:"mount"`
		Force bool   `json:"force"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !validMount(req.Mount) {
		jsonErr(w, http.StatusBadRequest, "invalid mount")
		return
	}
	data, err := a.Hub.Call(r.Context(), s.ID, "disk", "scan", map[string]any{"mount": req.Mount, "force": req.Force}, false, 125*time.Second)
	result := "ok"
	if err != nil {
		result = "failed"
	}
	a.audit(r, u, "disk.scan", "server", s.ID, s.Name, result, map[string]any{"mount": req.Mount})
	if err != nil {
		callErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

var diskCleanKinds = map[string]bool{"journal": true, "apt": true, "docker-images": true, "docker-build": true, "kernels": true,
	// Windows
	"win-temp": true, "win-update-cache": true, "win-recycle": true, "win-delivery": true, "win-dumps": true, "win-old": true}

// diskClean frees space in one of a few fixed ways; the agent works out what to remove itself.
func (a *Admin) diskClean(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !diskCleanKinds[req.Kind] {
		jsonErr(w, http.StatusBadRequest, "unknown kind")
		return
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	a.secCall(w, r, u, s, "disk", "clean", "disk.clean", map[string]any{"kind": req.Kind}, map[string]any{"kind": req.Kind}, 120*time.Second)
}

type confirmReq struct {
	Confirm bool   `json:"confirm"`
	Signal  string `json:"signal"`
}

func needConfirm(w http.ResponseWriter) {
	writeJSON(w, http.StatusConflict, map[string]any{"error": "confirmation required", "dangerous": true})
}

var unitNameRe = regexp.MustCompile(`^[a-zA-Z0-9:_.@-]{1,120}\.service$`)

// winServiceNameRe is the name of a Windows service: it has no ".service" suffix.
var winServiceNameRe = regexp.MustCompile(`^[a-zA-Z0-9:_.@$()-]{1,256}$`)

func (a *Admin) serviceAction(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	unit, action := r.PathValue("unit"), r.PathValue("action")
	win := isWindowsOS(s.OS)
	if (!win && !unitNameRe.MatchString(unit)) || (win && !winServiceNameRe.MatchString(unit)) {
		jsonErr(w, http.StatusBadRequest, "invalid unit")
		return
	}
	switch action {
	case "start", "stop", "restart", "enable", "disable":
	default:
		jsonErr(w, http.StatusBadRequest, "invalid action")
		return
	}
	var req confirmReq
	if !decode(w, r, &req) {
		return
	}
	danger := agent.IsDangerousService(unit, action)
	if win {
		danger = agent.IsDangerousWinService(unit, action)
	}
	if danger && !req.Confirm {
		needConfirm(w)
		return
	}
	_, err := a.Hub.Call(r.Context(), s.ID, "services", "action", map[string]string{"unit": unit, "action": action}, danger, 60*time.Second)
	res := "ok"
	if err != nil {
		res = "failed"
	}
	a.audit(r, u, "service."+action, "server", s.ID, s.Name, res, map[string]any{"unit": unit, "dangerous": danger})
	if err != nil {
		callErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Admin) reboot(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	var req confirmReq
	if !decode(w, r, &req) {
		return
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	_, err := a.Hub.Call(r.Context(), s.ID, "system", "reboot", nil, true, 20*time.Second)
	res := "ok"
	if err != nil {
		res = "failed"
	}
	a.audit(r, u, "server.reboot", "server", s.ID, s.Name, res, nil)
	if err != nil {
		callErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Admin) killProcess(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 1 {
		jsonErr(w, http.StatusBadRequest, "invalid pid")
		return
	}
	var req confirmReq
	if !decode(w, r, &req) {
		return
	}
	if !req.Confirm {
		needConfirm(w)
		return
	}
	if req.Signal != "KILL" {
		req.Signal = "TERM"
	}
	_, err = a.Hub.Call(r.Context(), s.ID, "processes", "kill", map[string]any{"pid": pid, "signal": req.Signal}, true, 20*time.Second)
	res := "ok"
	if err != nil {
		res = "failed"
	}
	a.audit(r, u, "process.kill", "server", s.ID, s.Name, res, map[string]any{"pid": pid, "signal": req.Signal})
	if err != nil {
		callErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- streams (terminal, logs, apt output) over WebSocket ----

var (
	logUnitRe = regexp.MustCompile(`^[a-zA-Z0-9:_.@*-]{1,100}$`)
	termLimit = 3 // simultaneous terminals per server
)

// termLimitCode is the WebSocket close code sent when the terminal limit for a server is reached.
const termLimitCode websocket.StatusCode = 4029

func atoiClamp(s string, lo, hi, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < lo || n > hi {
		return def
	}
	return n
}

func (a *Admin) streamParams(r *http.Request) (module string, params map[string]any, ok bool) {
	q := r.URL.Query()
	switch q.Get("kind") {
	case "terminal":
		p := map[string]any{"cols": atoiClamp(q.Get("cols"), 1, 500, 80), "rows": atoiClamp(q.Get("rows"), 1, 500, 24)}
		if user := q.Get("user"); user != "" { // a shell as another account (the Users tab); the agent checks it again
			if !termUserRe.MatchString(user) {
				return "", nil, false
			}
			p["user"] = user
		}
		return "terminal", p, true
	case "logs":
		p := map[string]any{"lines": atoiClamp(q.Get("lines"), 0, 1000, 200)}
		if u := q.Get("unit"); u != "" {
			if !logUnitRe.MatchString(u) {
				return "", nil, false
			}
			p["unit"] = u
		}
		if pr := q.Get("priority"); pr != "" {
			p["priority"] = strconv.Itoa(atoiClamp(pr, 0, 7, 6))
		}
		if q.Get("kernel") == "1" {
			p["kernel"] = "1"
		}
		return "logs", p, true
	case "winupdate":
		ids := []string{}
		if raw := q.Get("ids"); raw != "" {
			ids = strings.Split(raw, ",")
			if len(ids) > 200 {
				return "", nil, false
			}
			for _, id := range ids {
				if !winUpdateIDRe.MatchString(id) {
					return "", nil, false
				}
			}
		}
		return "winupdate", map[string]any{"ids": ids}, true
	case "docker-logs":
		if c := q.Get("container"); c != "" && len(c) <= 128 {
			return "docker.logs", map[string]any{"container": c, "lines": atoiClamp(q.Get("lines"), 0, 1000, 200)}, true
		}
	case "docker-pull":
		if img := q.Get("image"); img != "" && len(img) <= 256 {
			return "docker.pull", map[string]any{"image": img}, true
		}
	case "docker-update":
		if list := q.Get("containers"); list != "" {
			ids := strings.Split(list, ",")
			if len(ids) <= 100 && len(list) <= 4096 {
				return "docker.update", map[string]any{"containers": ids}, true
			}
			return "", nil, false
		}
		if c := q.Get("container"); c != "" && len(c) <= 128 && len(q.Get("image")) <= 256 {
			p := map[string]any{"container": c}
			if img := q.Get("image"); img != "" {
				p["image"] = img
			}
			if q.Get("project") == "1" {
				p["project"] = true
			}
			return "docker.update", p, true
		}
	case "docker-edit":
		if tok := q.Get("token"); len(tok) == 32 {
			if job := a.editJobs.take(r.PathValue("id"), tok); job != nil {
				return "docker.edit", map[string]any{"container": job.container, "spec": job.spec}, true
			}
		}
	case "docker-project-logs":
		if c := q.Get("container"); c != "" && len(c) <= 128 {
			return "docker.composeLogs", map[string]any{"container": c, "lines": atoiClamp(q.Get("lines"), 0, 1000, 200)}, true
		}
	case "f2b-install":
		preset := q.Get("preset")
		if preset == "soft" || preset == "norm" || preset == "hard" {
			return "security.f2bInstall", map[string]any{"preset": preset, "self": clientIP(r)}, true
		}
	case "apt":
		switch m := q.Get("mode"); m {
		case "update", "upgrade", "security":
			return "apt", map[string]any{"mode": m}, true
		case "packages":
			// the chosen packages: a comma separated list of apt names (a name cannot start a flag)
			names := []string{}
			for _, n := range strings.Split(q.Get("names"), ",") {
				if !aptName.MatchString(n) {
					return "", nil, false
				}
				names = append(names, n)
			}
			if len(names) > 0 && len(names) <= 500 {
				return "apt", map[string]any{"mode": "packages", "names": names}, true
			}
		}
	}
	return "", nil, false
}

// aptName is what a Debian package name may look like.
var aptName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.:~-]{0,127}$`)

func (a *Admin) streamWS(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	module, params, ok := a.streamParams(r)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "invalid stream parameters")
		return
	}
	if mod := streamModule(r.URL.Query().Get("kind")); mod != "" && u.Role != store.RoleAdmin && !a.moduleOpen(r, u, mod) {
		jsonErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if !a.Hub.Online(s.ID) {
		jsonErr(w, http.StatusServiceUnavailable, "agent is offline")
		return
	}
	if module == "terminal" && !a.termSlot(s.ID, +1) {
		// Browsers cannot read the status of a failed WebSocket handshake, so the limit is reported
		// as close code 4029 on an accepted socket (Accept answers non-WebSocket requests itself).
		if ws, err := websocket.Accept(w, r, nil); err == nil {
			_ = ws.Close(termLimitCode, "too many open terminals")
		}
		return
	}
	releaseTerm := func() {
		if module == "terminal" {
			a.termSlot(s.ID, -1)
		}
	}
	ws, err := websocket.Accept(w, r, nil) // same-origin check is on by default
	if err != nil {
		releaseTerm()
		return
	}
	defer releaseTerm()
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	sh, err := a.Hub.OpenStream(ctx, s.ID, module, params)
	if err != nil {
		_ = ws.Close(websocket.StatusInternalError, "cannot open stream")
		return
	}
	defer sh.Close()
	start := time.Now()
	var bytesIn, bytesOut int64
	auditParams := params
	if module == "docker.edit" { // the settings hold variable values: the journal gets the container only
		auditParams = map[string]any{"container": params["container"]}
	}
	a.audit(r, u, "stream.open", "server", s.ID, s.Name, "ok", map[string]any{"kind": module, "params": auditParams, "sid": sh.SID})
	defer func() {
		a.audit(r, u, "stream.close", "server", s.ID, s.Name, "ok", map[string]any{"kind": module, "sid": sh.SID,
			"seconds": int(time.Since(start).Seconds()), "bytes_in": bytesIn, "bytes_out": bytesOut})
	}()

	go func() { // browser -> agent
		defer cancel()
		for {
			typ, b, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if module != "terminal" {
				continue // logs and apt are output-only
			}
			if typ == websocket.MessageBinary {
				bytesIn += int64(len(b))
				if sh.Input(ctx, b) != nil {
					return
				}
				continue
			}
			var m struct {
				Resize *struct{ Cols, Rows int } `json:"resize"`
			}
			if json.Unmarshal(b, &m) == nil && m.Resize != nil {
				_ = sh.Resize(ctx, atoiClamp(strconv.Itoa(m.Resize.Cols), 1, 500, 80), atoiClamp(strconv.Itoa(m.Resize.Rows), 1, 500, 24))
			}
		}
	}()
	for { // agent -> browser
		select {
		case b := <-sh.Data:
			bytesOut += int64(len(b))
			if ws.Write(ctx, websocket.MessageBinary, b) != nil {
				return
			}
		case <-sh.Done():
			for { // flush what is already queued
				select {
				case b := <-sh.Data:
					_ = ws.Write(ctx, websocket.MessageBinary, b)
					continue
				default:
				}
				break
			}
			msg, _ := json.Marshal(map[string]string{"closed": sh.Reason()})
			_ = ws.Write(ctx, websocket.MessageText, msg)
			_ = ws.Close(websocket.StatusNormalClosure, "")
			return
		case <-ctx.Done():
			return
		}
	}
}

// termSlot counts open terminals per server; delta +1 reserves, -1 releases.
func (a *Admin) termSlot(serverID string, delta int) bool {
	a.termMu.Lock()
	defer a.termMu.Unlock()
	if delta > 0 {
		if a.terms[serverID] >= termLimit {
			return false
		}
		a.terms[serverID]++
		return true
	}
	if a.terms[serverID] > 0 {
		a.terms[serverID]--
	}
	return true
}
