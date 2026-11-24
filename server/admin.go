package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/internal/protocol"
	"github.com/bolgov0zero/linager/server/store"
)

const (
	cookieName = "linager_session"
	sessionTTL = 12 * time.Hour
	// appSessionTTL is the sliding lifetime of a session of the desktop application (the application port)
	appSessionTTL  = 7 * 24 * time.Hour
	maxEnrollTries = 5
)

type ctxKey int

const (
	keyUser ctxKey = iota
	keySession
)

// Admin serves the web admin panel and its JSON API.
type Admin struct {
	routes  []routeInfo
	DB      *store.Store
	Keys    *Keys
	Hub     *Hub
	Live    *Live
	Metrics *Metrics
	Updates *Updates
	Log     *slog.Logger
	Cfg     *config.Server
	Trust   *ProxyTrust
	Access  *AccessGuard
	Conn    *Connection // the ports (nil in tests that do not start the listeners)
	SSH     *SSHInstaller
	sshL    *ipLimiter
	Static  fs.FS // built frontend (may be nil in tests)

	loginL *ipLimiter

	termMu sync.Mutex
	terms  map[string]int
	desks  map[string]int // open remote desktops per server (guarded by termMu)

	assists    map[string]int                        // open Assistants per server and session (guarded by termMu)
	assistLive map[string]map[*StreamHandle]struct{} // the streams of the open Assistants per server (guarded by termMu)

	editJobs dockerEditJobs // settings of a container waiting for their stream (api_docker_edit.go)

	// passkey ceremonies in progress (the challenge must survive between begin and finish)
	cerMu sync.Mutex
	cers  map[string]*ceremony
}

// NewAdmin builds the admin handler set.
func NewAdmin(db *store.Store, keys *Keys, hub *Hub, live *Live, metrics *Metrics, updates *Updates, cfg *config.Server, static fs.FS, log *slog.Logger) (*Admin, error) {
	trust, err := NewProxyTrust(cfg.Admin.TrustedProxies)
	if err != nil {
		return nil, err
	}
	guard := newAccessGuard(db, trust, cfg.Admin.TrustedProxies, log)
	guard.loadHosts(context.Background()) // the last known addresses of host names, before the first request
	guard.Load(context.Background())
	ad := &Admin{DB: db, Keys: keys, Hub: hub, Live: live, Metrics: metrics, Updates: updates, Log: log, Cfg: cfg, Trust: trust, Access: guard, Static: static, loginL: newLimiter(max(cfg.Admin.LoginRate, 10), time.Minute), terms: map[string]int{}, cers: map[string]*ceremony{}, sshL: newLimiter(10, 10*time.Minute)}
	ad.SSH = ad.newSSHInstaller()
	return ad, nil
}

// Handler returns the full admin handler including the base path and security headers.
func (a *Admin) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", a.login)
	a.route(mux, "POST /api/logout", "any", a.logout)
	a.route(mux, "GET /api/me", "any", a.me)
	a.route(mux, "POST /api/me/totp/setup", "any", a.totpSetup)
	a.route(mux, "POST /api/me/totp/enable", "any", a.totpEnable)
	a.route(mux, "POST /api/me/totp/disable", "any", a.totpDisable)
	a.route(mux, "POST /api/me/password", "any", a.changeOwnPassword)
	a.route(mux, "POST /api/me/recovery-codes", "any", a.recoveryRegenerate)
	a.route(mux, "GET /api/me/passkeys", "any", a.passkeyList)
	a.route(mux, "POST /api/me/passkeys/begin", "any", a.passkeyRegisterBegin)
	a.route(mux, "POST /api/me/passkeys/finish", "any", a.passkeyRegisterFinish)
	a.route(mux, "DELETE /api/me/passkeys/{id}", "any", a.passkeyDelete)
	mux.HandleFunc("POST /api/login/passkey/begin", a.passkeyLoginBegin)
	mux.HandleFunc("POST /api/login/passkey/finish", a.passkeyLoginFinish)
	a.route(mux, "GET /api/users", "admin", a.usersList)
	a.route(mux, "POST /api/users", "admin", a.userCreate)
	a.route(mux, "DELETE /api/users/{id}", "admin", a.userDelete)
	a.route(mux, "PUT /api/users/{id}/access", "admin", a.userAccess)
	a.route(mux, "POST /api/users/{id}/password", "admin", a.userSetPassword)
	a.route(mux, "POST /api/users/{id}/2fa/reset", "admin", a.userReset2FA)
	a.route(mux, "GET /api/snippets", "mod:terminal", a.snippetsList)
	a.route(mux, "POST /api/snippets", "admin", a.snippetCreate)
	a.route(mux, "PUT /api/snippets/order", "admin", a.snippetsOrder)
	a.route(mux, "PUT /api/snippets/{id}", "admin", a.snippetUpdate)
	a.route(mux, "DELETE /api/snippets/{id}", "admin", a.snippetDelete)
	a.route(mux, "POST /api/snippets/{id}/ran", "mod:terminal", a.snippetRan)
	a.route(mux, "GET /api/enrollments", "admin", a.listEnrollments)
	a.route(mux, "POST /api/enrollments/{id}/approve", "admin", a.approve)
	a.route(mux, "POST /api/enrollments/{id}/reject", "admin", a.reject)
	a.route(mux, "GET /api/servers", "any", a.listServers)
	a.route(mux, "DELETE /api/servers/{id}", "srv:admin", a.deleteServer)
	a.route(mux, "GET /api/servers/{id}", "srv", a.serverDetail)
	a.route(mux, "GET /api/servers/{id}/metrics", "srv", a.history)
	a.route(mux, "GET /api/servers/{id}/metrics.csv", "srv", a.historyCSV)
	a.route(mux, "GET /api/connect", "admin", a.connectInfo)
	a.route(mux, "GET /api/join-tokens", "admin", a.joinList)
	a.route(mux, "POST /api/join-tokens", "admin", a.joinCreate)
	a.route(mux, "DELETE /api/join-tokens/{id}", "admin", a.joinRevoke)
	a.route(mux, "POST /api/ssh-install", "admin", a.sshStart)
	a.route(mux, "GET /api/ssh-install/{id}", "admin", a.sshGet)
	a.route(mux, "POST /api/ssh-install/{id}/answer", "admin", a.sshAnswer)
	a.route(mux, "POST /api/ssh-install/{id}/install", "admin", a.sshInstall)
	a.route(mux, "DELETE /api/ssh-install/{id}", "admin", a.sshClose)
	a.route(mux, "GET /api/groups", "any", a.groupsList)
	a.route(mux, "POST /api/groups", "admin", a.groupCreate)
	a.route(mux, "PUT /api/groups/order", "admin", a.groupsReorder)
	a.route(mux, "PUT /api/groups/{id}", "admin", a.groupRename)
	a.route(mux, "DELETE /api/groups/{id}", "admin", a.groupDelete)
	a.route(mux, "PUT /api/servers/{id}/group", "srv:admin", a.serverSetGroup)
	a.route(mux, "PUT /api/servers/{id}/note", "srv:admin", a.serverNoteSet)
	a.route(mux, "GET /api/dashboard", "any", a.dashboard)
	a.route(mux, "GET /api/dashboard/available", "admin", a.dashboardAvailable)
	a.route(mux, "POST /api/dashboard/items", "admin", a.dashboardAdd)
	a.route(mux, "DELETE /api/dashboard/items/{id}", "admin", a.dashboardRemove)
	a.route(mux, "PUT /api/dashboard/order", "any", a.dashboardOrder)
	a.route(mux, "GET /api/live", "any", a.liveHandler)
	a.route(mux, "GET /api/updates", "admin", a.updatesGet)
	a.route(mux, "GET /api/updates/summary", "admin", a.updatesSummary)
	a.route(mux, "POST /api/updates/check", "admin", a.updatesCheck)
	a.route(mux, "PUT /api/updates/channel", "admin", a.updatesChannel)
	a.route(mux, "POST /api/updates/cache", "admin", a.updatesCache)
	a.route(mux, "POST /api/updates/cache/prune", "admin", a.updatesCachePrune)
	a.route(mux, "POST /api/updates/server", "admin", a.updatesServer)
	a.route(mux, "POST /api/updates/agents", "admin", a.updatesAgents)
	a.route(mux, "GET /api/audit", "admin", a.auditList)
	a.route(mux, "GET /api/connection", "admin", a.connGet)
	a.route(mux, "PUT /api/connection", "admin", a.connPut)
	a.route(mux, "POST /api/connection/close-old", "admin", a.connCloseOld)
	a.route(mux, "GET /api/access", "admin", a.accessGet)
	a.route(mux, "PUT /api/access", "admin", a.accessPut)
	a.route(mux, "POST /api/access/check", "admin", a.accessCheck)
	a.route(mux, "POST /api/access/resolve", "admin", a.accessResolve)
	a.route(mux, "PUT /api/access/retention", "admin", a.accessRetention)
	a.route(mux, "GET /api/access/log", "admin", a.accessLog)
	a.route(mux, "GET /api/servers/{id}/services", "srv:services", a.servicesList)
	a.route(mux, "GET /api/servers/{id}/services-ignored", "srv:services", a.servicesIgnored)
	a.route(mux, "PUT /api/servers/{id}/services-ignored", "srv:services", a.servicesIgnore)
	a.route(mux, "POST /api/servers/{id}/services/{unit}/{action}", "srv:services", a.serviceAction)
	a.route(mux, "GET /api/servers/{id}/apt", "srv:apt", a.aptList)
	a.route(mux, "PUT /api/servers/{id}/apt-ignore", "srv:apt", a.ignoreUpdates("apt"))
	a.route(mux, "POST /api/servers/{id}/reboot", "srv:admin", a.reboot)
	a.route(mux, "GET /api/servers/{id}/disk", "srv:disk", a.diskState)
	a.route(mux, "POST /api/servers/{id}/disk/scan", "srv:disk", a.diskScan)
	a.route(mux, "POST /api/servers/{id}/disk/clean", "srv:disk", a.diskClean)
	a.route(mux, "GET /api/servers/{id}/security", "srv:security", a.securityState)
	a.route(mux, "POST /api/servers/{id}/security/terminate", "srv:security", a.securityTerminate)
	a.route(mux, "POST /api/servers/{id}/security/ban", "srv:security", a.securityBan)
	a.route(mux, "POST /api/servers/{id}/security/unban", "srv:security", a.securityUnban)
	a.route(mux, "PUT /api/servers/{id}/security/fail2ban", "srv:security", a.securityF2bSettings)
	a.route(mux, "GET /api/servers/{id}/security/bans", "srv:security", a.securityBanHistory)
	a.route(mux, "POST /api/servers/{id}/security/keys/remove", "srv:security", a.securityKeyRemove)
	a.route(mux, "GET /api/servers/{id}/users", "srv:users", a.hostUsersState)
	a.route(mux, "POST /api/servers/{id}/users", "srv:users", a.hostUserAdd)
	a.route(mux, "PUT /api/servers/{id}/users/{name}", "srv:users", a.hostUserUpdate)
	a.route(mux, "POST /api/servers/{id}/users/{name}/delete", "srv:users", a.hostUserDelete)
	a.route(mux, "POST /api/servers/{id}/users/{name}/password", "srv:users", a.hostUserPassword)
	a.route(mux, "POST /api/servers/{id}/users/{name}/kill", "srv:users", a.hostUserKill)
	a.route(mux, "GET /api/servers/{id}/processes", "srv:processes", a.processesList)
	a.route(mux, "POST /api/servers/{id}/processes/{pid}/kill", "srv:processes", a.killProcess)
	a.route(mux, "GET /api/servers/{id}/stream", "srv", a.streamWS)
	a.route(mux, "POST /api/servers/{id}/win/{module}/{action}", "srv", a.winCall) // the rights are checked by the action
	a.route(mux, "GET /api/servers/{id}/desktop", "srv:desktop", a.desktopWS)
	a.route(mux, "GET /api/servers/{id}/desktop/state", "srv:desktop", a.desktopState)
	a.route(mux, "POST /api/servers/{id}/desktop/enable", "srv:desktop", a.desktopEnable)
	a.route(mux, "POST /api/servers/{id}/desktop/disable", "srv:desktop", a.desktopDisable)
	a.route(mux, "GET /api/servers/{id}/assistant", "srv:assistant", a.assistantWS)
	a.route(mux, "GET /api/servers/{id}/assistant/sessions", "srv:assistant", a.assistantSessions)
	a.route(mux, "POST /api/servers/{id}/assistant/sas", "srv:assistant", a.assistantSas)
	a.route(mux, "GET /api/servers/{id}/assistant/settings", "srv:assistant", a.assistantSettingsGet)
	a.route(mux, "PUT /api/servers/{id}/assistant/settings", "srv:assistant", a.assistantSettingsSet)
	a.route(mux, "GET /api/servers/{id}/files", "srv:files", a.filesList)
	a.route(mux, "GET /api/servers/{id}/files/content", "srv:files", a.filesContent)
	a.route(mux, "PUT /api/servers/{id}/files/content", "srv:files", a.simpleFileOp("write", "files.write", false))
	a.route(mux, "POST /api/servers/{id}/files/mkdir", "srv:files", a.simpleFileOp("mkdir", "files.mkdir", false))
	a.route(mux, "POST /api/servers/{id}/files/create", "srv:files", a.simpleFileOp("create", "files.create", false))
	a.route(mux, "POST /api/servers/{id}/files/rename", "srv:files", a.fileRename)
	a.route(mux, "GET /api/servers/{id}/files/stat", "srv:files", a.fileStat)
	a.route(mux, "GET /api/servers/{id}/files/du", "srv:files", a.fileDu)
	a.route(mux, "POST /api/servers/{id}/files/chmod", "srv:files", a.filePerm("chmod"))
	a.route(mux, "POST /api/servers/{id}/files/chown", "srv:files", a.filePerm("chown"))
	a.route(mux, "POST /api/servers/{id}/files/copy", "srv:files", a.fileTransfer("copy"))
	a.route(mux, "POST /api/servers/{id}/files/move", "srv:files", a.fileTransfer("move"))
	a.route(mux, "GET /api/servers/{id}/files/zip", "srv:files", a.fileZip)
	a.route(mux, "POST /api/servers/{id}/files/delete", "srv:files", a.simpleFileOp("delete", "files.delete", true))
	a.route(mux, "GET /api/servers/{id}/files/download", "srv:files", a.fileDownload)
	a.route(mux, "POST /api/servers/{id}/files/upload", "srv:files", a.fileUpload)
	a.route(mux, "GET /api/servers/{id}/network", "srv:network", a.networkState)
	a.route(mux, "POST /api/servers/{id}/network/validate", "srv:network", a.networkValidate)
	a.route(mux, "PUT /api/servers/{id}/network/draft", "srv:network", a.networkSaveDraft)
	a.route(mux, "DELETE /api/servers/{id}/network/draft", "srv:network", a.resetDraft("network"))
	a.route(mux, "POST /api/servers/{id}/network/apply", "srv:network", a.networkApply)
	a.route(mux, "POST /api/servers/{id}/network/config", "srv:network", a.networkApplyConfig)
	a.route(mux, "POST /api/servers/{id}/network/dns", "srv:network", a.networkApplyDNS)
	a.route(mux, "POST /api/servers/{id}/network/confirm", "srv:network", a.confirmOrRollback("network", "confirm"))
	a.route(mux, "POST /api/servers/{id}/network/rollback", "srv:network", a.confirmOrRollback("network", "rollback"))
	a.route(mux, "GET /api/servers/{id}/firewall", "srv:firewall", a.firewallState)
	a.route(mux, "POST /api/servers/{id}/firewall/validate", "srv:firewall", a.firewallValidate)
	a.route(mux, "GET /api/servers/{id}/firewall/ports", "srv:firewall", a.firewallPorts)
	a.route(mux, "GET /api/servers/{id}/firewall/log", "srv:firewall", a.firewallLog)
	a.route(mux, "PUT /api/servers/{id}/firewall/draft", "srv:firewall", a.firewallSaveDraft)
	a.route(mux, "DELETE /api/servers/{id}/firewall/draft", "srv:firewall", a.resetDraft("firewall"))
	a.route(mux, "POST /api/servers/{id}/firewall/apply", "srv:firewall", a.firewallApply)
	a.route(mux, "POST /api/servers/{id}/firewall/confirm", "srv:firewall", a.confirmOrRollback("firewall", "confirm"))
	a.route(mux, "POST /api/servers/{id}/firewall/rollback", "srv:firewall", a.confirmOrRollback("firewall", "rollback"))
	a.route(mux, "GET /api/servers/{id}/docker", "srv:docker", a.dockerState)
	a.route(mux, "GET /api/servers/{id}/docker/containers/{cid}/config", "srv:docker", a.dockerConfig)
	a.route(mux, "POST /api/servers/{id}/docker/containers/{cid}/edit", "srv:docker", a.dockerEdit)
	a.route(mux, "POST /api/servers/{id}/docker/containers/{cid}/{action}", "srv:docker", a.dockerContainerAction)
	a.route(mux, "DELETE /api/servers/{id}/docker/images/{iid}", "srv:docker", a.dockerRemoveImage)
	a.route(mux, "POST /api/servers/{id}/docker/check", "srv:docker", a.dockerCheck)
	a.route(mux, "PUT /api/servers/{id}/docker-ignore", "srv:docker", a.ignoreUpdates("docker"))
	if a.Static != nil {
		mux.Handle("/", a.static())
	}
	var h http.Handler = mux
	if bp := strings.TrimRight(a.Cfg.Admin.BasePath, "/"); bp != "" {
		h = http.StripPrefix(bp, mux)
	}
	return a.secureHeaders(a.accessGuard(h))
}

func (a *Admin) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if a.Trust.IsHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		// The remote desktop client is WebAssembly (it needs wasm-unsafe-eval, which allows no script from a string) and
		// loads its module from a data: address, so connect-src lists data: as well.
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; connect-src 'self' data:; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Admin) static() http.Handler {
	// Not every system mime table knows the web app manifest; nosniff is set, so the type must be right.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	files := http.FileServerFS(a.Static)
	gz := &gzipAssets{fsys: a.Static}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gz.serve(w, r) {
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(a.Static, p); err != nil {
				// A script or style that is gone (a page opened before an update asks for the old file) must be a
				// 404: the page of the application in its place cannot be loaded as a module and the window that
				// needed it stays empty.
				if strings.HasPrefix(p, "assets/") {
					http.NotFound(w, r)
					return
				}
				r.URL.Path = "/" // SPA fallback
				p = ""
			}
		}
		// The page itself names the files of this build: it is always asked again, so a new release is picked up.
		if p == "" || p == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// ---- helpers ----

func jsonErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v); err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request")
		return false
	}
	return true
}

func (a *Admin) audit(r *http.Request, u *store.User, action, ttype, tid, tname, result string, details map[string]any) {
	au := store.Audit{Action: action, TargetType: ttype, TargetID: tid, TargetName: tname, Result: result, Details: details,
		RemoteAddr: remoteAddr(r).String(), ClientIP: a.Trust.ClientIP(r)}
	if u != nil {
		au.UserID, au.Actor = &u.ID, u.Login
	}
	if err := a.DB.WriteAudit(context.WithoutCancel(r.Context()), au); err != nil {
		a.Log.Error("audit write failed", "action", action, "err", err)
	}
	if action == "auth.login" {
		login := tname
		if u != nil {
			login = u.Login
		}
		a.Access.logLogin(r, a.Access.snap.Load().rules.decide(remoteAddr(r), r.Header.Get("X-Forwarded-For")), login, result)
	}
}

func (a *Admin) secureCookie(r *http.Request) bool {
	switch a.Cfg.Admin.CookieSecure {
	case "always":
		return true
	case "never":
		return false
	}
	return a.Trust.IsHTTPS(r)
}

// auth requires a valid session; non-GET requests also need a matching CSRF token.
func (a *Admin) auth(next func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		se, err := a.DB.SessionByHash(r.Context(), HashToken(c.Value))
		if err != nil {
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(se.CSRF)) != 1 {
				jsonErr(w, http.StatusForbidden, "bad csrf token")
				return
			}
		}
		u, err := a.DB.UserByID(r.Context(), se.UserID)
		if err != nil {
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if u.Role, err = a.DB.RoleOf(r.Context(), u.ID); err != nil {
			jsonErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		if a.Cfg.Admin.RequireTOTP && !u.TOTPEnabled && !totpSetupPath(r.URL.Path) {
			jsonErr(w, http.StatusForbidden, "2fa_required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), keySession, se)), u)
	}
}

// ---- auth endpoints ----

// ErrTOTPRequired is the error code of the sign-in answer that asks for the 2FA code (second stage).
const ErrTOTPRequired = "totp_required"

type loginReq struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	TOTP     string `json:"totp"`
}

func (a *Admin) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decode(w, r, &req) {
		return
	}
	ip := a.Trust.ClientIP(r)
	if !a.loginL.Allow(ip) || !a.loginL.Allow("peer:"+remoteAddr(r).String()) {
		a.audit(r, nil, "auth.login", "user", "", req.Login, "rate_limited", nil)
		jsonErr(w, http.StatusTooManyRequests, "too many attempts, try later")
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.Login))
	u, err := a.DB.UserByLogin(r.Context(), login)
	if err != nil {
		VerifyPassword(req.Password, dummyHash) // equalize timing
		a.audit(r, nil, "auth.login", "user", "", login, "failed", nil)
		jsonErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		a.audit(r, u, "auth.login", "user", "", login, "locked", nil)
		jsonErr(w, http.StatusTooManyRequests, "account is temporarily locked")
		return
	}
	ok := VerifyPassword(req.Password, u.PasswordHash)
	method := "password"
	if ok && u.TOTPEnabled && strings.TrimSpace(req.TOTP) == "" {
		// first stage of a two-step sign-in: the password is right, the browser must ask for the 2FA
		// code and repeat the request with it. This is not a failed attempt and creates no session.
		jsonErr(w, http.StatusUnauthorized, ErrTOTPRequired)
		return
	}
	if ok && u.TOTPEnabled {
		if isTOTPCode(req.TOTP) {
			step, err := MatchTOTP(u.TOTPSecret, req.TOTP, time.Now())
			if err != nil {
				ok = false
			} else if fresh, err := a.DB.AdvanceTOTPStep(r.Context(), u.ID, step); err != nil || !fresh {
				ok = false // code already used
			}
			method = "password+totp"
		} else {
			used, err := a.DB.UseRecoveryCode(r.Context(), u.ID, HashRecoveryCode(req.TOTP))
			ok = err == nil && used
			method = "password+recovery"
		}
	}
	if !ok {
		_ = a.DB.RecordLoginFailure(r.Context(), u.ID, lockDuration(u.FailedAttempts+1))
		a.audit(r, u, "auth.login", "user", "", login, "failed", nil)
		jsonErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	a.finishLogin(w, r, u, method)
}

// finishLogin clears the failure counter, opens the session and answers with the session data.
func (a *Admin) finishLogin(w http.ResponseWriter, r *http.Request, u *store.User, method string) {
	_ = a.DB.RecordLoginSuccess(r.Context(), u.ID)
	token, hash := NewToken()
	csrf := NewCSRF()
	ttl, sliding := sessionTTL, false
	if isDirect(r) { // the application port: seven days, renewed by every request
		ttl, sliding = appSessionTTL, true
	}
	if err := a.DB.CreateSession(r.Context(), hash, u.ID, csrf, a.Trust.ClientIP(r), r.UserAgent(), ttl, sliding); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, //nolint:gosec // Secure follows cookie_secure/forwarded proto: plain-HTTP LANs are supported
		Path: "/", HttpOnly: true, Secure: a.secureCookie(r),
		SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds())})
	a.audit(r, u, "auth.login", "user", "", u.Login, "ok", map[string]any{"method": method})
	writeJSON(w, http.StatusOK, a.sessionBody(r, u, csrf))
}

// sessionBody is the answer of login and me.
func (a *Admin) sessionBody(r *http.Request, u *store.User, csrf string) map[string]any {
	n, _ := a.DB.PasskeyCount(r.Context(), u.ID)
	left, _ := a.DB.RecoveryRemaining(r.Context(), u.ID)
	if u.Role == "" {
		u.Role, _ = a.DB.RoleOf(r.Context(), u.ID)
	}
	modules := []string{}
	switch u.Role {
	case store.RoleAdmin:
		modules = append(modules, OperatorModules...)
	case store.RoleOperator:
		if acc, err := a.DB.UserAccess(r.Context(), u.ID); err == nil {
			modules = acc.Modules
		}
	}
	return map[string]any{"csrf": csrf, "login": u.Login, "id": u.ID, "totp_enabled": u.TOTPEnabled, "passkeys": n, "recovery_left": left,
		"totp_required": a.Cfg.Admin.RequireTOTP && !u.TOTPEnabled, "role": u.Role, "modules": modules}
}

func (a *Admin) logout(w http.ResponseWriter, r *http.Request, u *store.User) {
	if c, err := r.Cookie(cookieName); err == nil {
		_ = a.DB.DeleteSession(r.Context(), HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode}) //nolint:gosec // clearing cookie
	a.audit(r, u, "auth.logout", "user", "", u.Login, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Admin) me(w http.ResponseWriter, r *http.Request, u *store.User) {
	se := r.Context().Value(keySession).(*store.Session)
	writeJSON(w, http.StatusOK, a.sessionBody(r, u, se.CSRF))
}

// connectInfo returns what a client needs to connect to this server: the agent port, the fingerprint of the agent-port
// certificate and the names/IPs that certificate was issued for. The agent port is not behind the admin reverse proxy,
// so the page picks the host itself (usually the one the administrator opened the panel with).
func (a *Admin) connectInfo(w http.ResponseWriter, _ *http.Request, _ *store.User) {
	port := a.agentPort()
	hosts := a.Cfg.Agents.Hosts
	if hosts == nil {
		hosts = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"port": port, "fingerprint": pki.Fingerprint(a.Keys.TLSDER), "hosts": hosts})
}

func (a *Admin) totpSetup(w http.ResponseWriter, r *http.Request, u *store.User) {
	if u.TOTPEnabled {
		jsonErr(w, http.StatusConflict, "2FA is already enabled")
		return
	}
	secret, uri, err := NewTOTPSecret(u.Login)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := a.DB.SetTOTP(r.Context(), u.ID, secret, false); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": uri})
}

type codeReq struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

func (a *Admin) totpEnable(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req codeReq
	if !decode(w, r, &req) {
		return
	}
	if u.TOTPSecret == "" || u.TOTPEnabled {
		jsonErr(w, http.StatusConflict, "run setup first")
		return
	}
	step, err := MatchTOTP(u.TOTPSecret, req.Code, time.Now())
	if err != nil {
		jsonErr(w, http.StatusUnprocessableEntity, "invalid code")
		return
	}
	if err := a.DB.SetTOTP(r.Context(), u.ID, u.TOTPSecret, true); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	_, _ = a.DB.AdvanceTOTPStep(r.Context(), u.ID, step)
	// the recovery codes are shown once, right now
	codes, hashes := NewRecoveryCodes()
	if err := a.DB.ReplaceRecoveryCodes(r.Context(), u.ID, hashes); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "auth.totp.enable", "user", "", u.Login, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recovery_codes": codes})
}

func (a *Admin) totpDisable(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req codeReq
	if !decode(w, r, &req) {
		return
	}
	if !VerifyPassword(req.Password, u.PasswordHash) {
		a.audit(r, u, "auth.totp.disable", "user", "", u.Login, "failed", nil)
		jsonErr(w, http.StatusForbidden, "invalid password")
		return
	}
	if err := a.DB.SetTOTP(r.Context(), u.ID, "", false); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	_ = a.DB.ReplaceRecoveryCodes(r.Context(), u.ID, nil)
	a.audit(r, u, "auth.totp.disable", "user", "", u.Login, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- enrollments ----

type enrollmentView struct {
	ID           string    `json:"id"`
	Hostname     string    `json:"hostname"`
	Address      string    `json:"address"`
	RemoteAddr   string    `json:"remote_addr"`
	OS           string    `json:"os"`
	AgentVersion string    `json:"agent_version"`
	KeyFP        string    `json:"key_fingerprint"`
	VerifyCode   string    `json:"verify_code"`
	ExpiresAt    time.Time `json:"expires_at"`
	// Via is the label of the install link the agent came with; such a request needs no key.
	Via      string `json:"via,omitempty"`
	NeedsKey bool   `json:"needs_key"`
}

func (a *Admin) listEnrollments(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := a.DB.PendingEnrollments(r.Context())
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]enrollmentView, 0, len(list))
	for _, e := range list {
		via, needsKey := "", true
		if t := a.joinOf(r.Context(), e); t != nil {
			via, needsKey = t.Label, false
		}
		out = append(out, enrollmentView{
			ID: e.ID, Hostname: e.Hostname, Address: e.Address, RemoteAddr: e.RemoteAddr, OS: e.OS, AgentVersion: e.AgentVersion,
			KeyFP:      pki.ShortFingerprint(pki.KeyFingerprint(e.AgentPubkey)),
			VerifyCode: pki.VerificationCode(a.Keys.TLSDER, e.AgentPubkey, e.Nonce),
			ExpiresAt:  e.ExpiresAt, Via: via, NeedsKey: needsKey,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *Admin) approve(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	e, err := a.DB.Enrollment(r.Context(), id)
	if err != nil || e.Status != "pending" || time.Now().After(e.ExpiresAt) {
		jsonErr(w, http.StatusNotFound, "request not found or expired")
		return
	}
	// An agent that came with a valid install link needs no key: the link made by an administrator says it is theirs.
	tok := a.joinOf(r.Context(), e)
	want := pki.EnrollmentKey(e.AgentPubkey)
	got := pki.NormalizeKey(req.Key)
	if tok == nil && (got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1) {
		n, _ := a.DB.BumpEnrollmentAttempts(r.Context(), id, maxEnrollTries)
		a.audit(r, u, "enroll.approve", "enrollment", id, e.Hostname, "bad_key", map[string]any{"attempts": n})
		if n >= maxEnrollTries {
			jsonErr(w, http.StatusGone, "too many wrong keys, the request was rejected")
			return
		}
		jsonErr(w, http.StatusUnprocessableEntity, "wrong key")
		return
	}
	srv, err := a.DB.ApproveEnrollment(r.Context(), e, func(serverID string) (string, error) {
		pem, err := a.Keys.CA.IssueAgentCert(e.AgentPubkey, serverID, a.Cfg.AgentCertLifetime())
		return string(pem), err
	})
	if err != nil {
		a.Log.Error("approve enrollment", "err", err)
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	details := map[string]any{"address": e.Address}
	if tok != nil {
		details["link"] = tok.Label
		if tok.GroupID != nil {
			_ = a.DB.SetServerGroup(r.Context(), srv.ID, tok.GroupID)
		}
	}
	a.audit(r, u, "enroll.approve", "server", srv.ID, srv.Name, "ok", details)
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, srv)
}

func (a *Admin) reject(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if err := a.DB.RejectEnrollment(r.Context(), id); err != nil {
		jsonErr(w, http.StatusNotFound, "request not found")
		return
	}
	a.audit(r, u, "enroll.reject", "enrollment", id, "", "ok", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- servers ----

type serverView struct {
	*store.Server
	Online  bool   `json:"online"`
	GroupID *int64 `json:"group_id"`
}

func (a *Admin) listServers(w http.ResponseWriter, r *http.Request, u *store.User) {
	list, err := a.DB.Servers(r.Context())
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	visible, err := a.visibleServers(r, u)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	groups, _ := a.DB.ServerGroups(r.Context())
	out := make([]serverView, 0, len(list))
	for _, s := range list {
		if visible != nil && !visible[s.ID] {
			continue
		}
		v := serverView{Server: s, Online: a.Hub.Online(s.ID)}
		if g, ok := groups[s.ID]; ok {
			v.GroupID = &g
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteServer removes a client from the server. The caller must repeat the server name
// (protection against mis-clicks). With uninstall=true and the agent online, the agent is
// first told (signed command) to remove itself from its host.
func (a *Admin) deleteServer(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Name      string `json:"name"`
		Uninstall bool   `json:"uninstall"`
	}
	if !decode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	srv, err := a.DB.ServerByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonErr(w, http.StatusNotFound, "server not found")
		} else {
			jsonErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if req.Name != srv.Name {
		jsonErr(w, http.StatusUnprocessableEntity, "confirmation name does not match")
		return
	}
	online := a.Hub.Online(id)
	sent := false
	if req.Uninstall && online {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		sent = a.Hub.Send(ctx, id, protocol.TypeAgentRemove, protocol.Uninstall{Purge: true}) == nil
		cancel()
		if sent {
			time.Sleep(500 * time.Millisecond) // let the frame leave before the connection is closed
		}
	}
	if err := a.DB.DeleteServer(r.Context(), id); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.Hub.Kick(id)
	a.Metrics.Forget(id)
	a.Live.Publish("dashboard", map[string]any{})
	a.audit(r, u, "server.delete", "server", id, srv.Name, "ok", map[string]any{"was_online": online, "uninstall_requested": req.Uninstall, "uninstall_sent": sent})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "uninstall_sent": sent, "was_online": online})
}

// bg returns a context for work that must outlive a request.
func (a *Admin) bg() context.Context { return context.Background() }

// totpSetupPath lists the endpoints usable before 2FA is enabled when it is required.
func totpSetupPath(p string) bool {
	switch p {
	case "/api/me", "/api/logout", "/api/me/totp/setup", "/api/me/totp/enable":
		return true
	}
	return false
}

// agentPort is the port the agents connect to now (the one set in the panel wins over server.yaml).
func (a *Admin) agentPort() string {
	if a.Conn != nil {
		return strconv.Itoa(a.Conn.State().AgentPort)
	}
	_, p, _ := net.SplitHostPort(a.Cfg.Agents.Listen)
	return p
}
