package server

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bolgov0zero/linager/server/store"
)

// Devices. Every application that signs in (the desktop application, the iPhone application, later Android) sends a secret token of
// its own (X-Linager-Device, made when it first starts; only its hash is kept) and says what it is (X-Linager-Client). The server
// keeps a device for each, shows them in Settings, "Devices", and, when the administrator turns the confirmation on, lets a new
// device in only after an administrator approves it. Until then the device may only ask who it is and sign out.

const (
	headerDevice = "X-Linager-Device"
	headerClient = "X-Linager-Client"

	// ErrApprovalPending, ErrDeviceDenied and ErrDeviceRequired are the codes of the answers that keep a device out.
	ErrApprovalPending = "approval_pending"
	ErrDeviceDenied    = "device_denied"
	ErrDeviceRequired  = "device_required"

	gateCacheTTL = 3 * time.Second
)

// deviceGate caches the setting that turns the confirmation on, so that the command line of the server can turn it off without a restart.
type deviceGate struct {
	mu    sync.Mutex
	at    time.Time
	since time.Time
}

// gateSince is the time the confirmation of devices was turned on; zero when it is off.
func (a *Admin) gateSince(ctx context.Context) time.Time {
	a.gate.mu.Lock()
	defer a.gate.mu.Unlock()
	if time.Since(a.gate.at) < gateCacheTTL {
		return a.gate.since
	}
	a.gate.since = parseGate(a.DB.Setting(ctx, store.DeviceApprovalSetting))
	a.gate.at = time.Now()
	return a.gate.since
}

func (a *Admin) gateReset() {
	a.gate.mu.Lock()
	a.gate.at = time.Time{}
	a.gate.mu.Unlock()
}

func parseGate(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Now() // a value that cannot be read is taken as "on, since now": the safe side
	}
	return t
}

// deviceToken reads the token of the device from the request and returns its hash.
func deviceToken(r *http.Request) ([]byte, bool) {
	t := strings.TrimSpace(r.Header.Get(headerDevice))
	if len(t) < 32 || len(t) > 128 {
		return nil, false
	}
	if _, err := base64.RawURLEncoding.DecodeString(t); err != nil {
		return nil, false
	}
	return HashToken(t), true
}

// clientInfo reads what the client says about itself: a query string, "platform=ios&app=0.1.0&os=27.1&model=iPhone18%2C1".
func clientInfo(r *http.Request) store.DeviceInfo {
	q, err := url.ParseQuery(r.Header.Get(headerClient))
	if err != nil {
		return store.DeviceInfo{}
	}
	lim := func(s string) string {
		s = strings.TrimSpace(s)
		if r := []rune(s); len(r) > 64 {
			s = string(r[:64])
		}
		return s
	}
	info := store.DeviceInfo{Platform: strings.ToLower(lim(q.Get("platform"))), AppVersion: lim(q.Get("app")), OSVersion: lim(q.Get("os")), Model: lim(q.Get("model"))}
	switch info.Platform {
	case "ios", "android", "windows", "macos", "linux", "":
	default:
		info.Platform = "other"
	}
	return info
}

// pendingMay tells what a device that waits for approval may still ask: who it is, and to sign out.
func pendingMay(r *http.Request) bool {
	return (r.Method == http.MethodGet && r.URL.Path == "/api/me") || (r.Method == http.MethodPost && r.URL.Path == "/api/logout")
}

// deviceCheck is the part of the authentication that is about the device. It returns the device of the request (nil when the client
// has none) and false when it has already answered. since is zero when the confirmation is off.
func (a *Admin) deviceCheck(w http.ResponseWriter, r *http.Request, se *store.Session, u *store.User, since time.Time) (*store.Device, bool) {
	ctx := r.Context()
	tok, hasTok := deviceToken(r)
	info := clientInfo(r)
	ip := clientIP(r)
	on := !since.IsZero()

	var d *store.Device
	if se.DeviceID == nil {
		switch {
		case !hasTok && !on:
			return nil, true // a client that has no device (an old one), and nothing is confirmed
		case !hasTok:
			jsonErr(w, http.StatusForbidden, ErrDeviceRequired)
			return nil, false
		case on && !se.Created.Before(since):
			// a session that was opened after the confirmation was turned on has its device from the sign-in
			jsonErr(w, http.StatusForbidden, ErrDeviceRequired)
			return nil, false
		}
		// a session that was open before the device was known (or before the confirmation was turned on, which approved it): the first
		// request with a device ties it
		dev, created, err := a.DB.RegisterDevice(ctx, u.ID, tok, info, ip, store.DeviceApproved)
		if err != nil {
			jsonErr(w, http.StatusInternalServerError, "internal error")
			return nil, false
		}
		if err := a.DB.BindSession(ctx, se.ID, dev.ID); err != nil {
			jsonErr(w, http.StatusInternalServerError, "internal error")
			return nil, false
		}
		if created {
			a.Live.PublishAdmin("devices", map[string]any{"id": dev.ID})
		}
		d = dev
	} else {
		dev, err := a.DB.DeviceByID(ctx, *se.DeviceID)
		if err != nil {
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return nil, false
		}
		// the cookie alone is not enough once the device is confirmed: the request must come with the token of the device
		if on && (!hasTok || subtle.ConstantTimeCompare(tok, dev.TokenHash) != 1) {
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return nil, false
		}
		d = dev
		fresh := time.Since(d.LastSeen) < time.Minute && d.LastIP == ip &&
			(info.AppVersion == "" || info.AppVersion == d.AppVersion) && (info.OSVersion == "" || info.OSVersion == d.OSVersion)
		if !fresh {
			if err := a.DB.TouchDevice(ctx, d.ID, info, ip); err == nil {
				d.LastIP, d.LastSeen = ip, time.Now()
			}
		}
	}
	if on {
		switch d.Status {
		case store.DeviceDenied:
			jsonErr(w, http.StatusForbidden, ErrDeviceDenied)
			return nil, false
		case store.DevicePending:
			if !pendingMay(r) {
				jsonErr(w, http.StatusForbidden, ErrApprovalPending)
				return nil, false
			}
		}
	}
	return d, true
}

// deviceOfRequest is the device of the request, as deviceCheck found it (nil: none).
func deviceOfRequest(r *http.Request) *store.Device {
	d, _ := r.Context().Value(keyDevice).(*store.Device)
	return d
}

// deviceBody is what the sign-in and "who am I" tell the client about its device.
func (a *Admin) deviceBody(r *http.Request, d *store.Device) map[string]any {
	on := !a.gateSince(r.Context()).IsZero()
	out := map[string]any{"confirm": on}
	if d != nil {
		out["id"] = d.ID
		out["status"] = d.Status
		if !on {
			out["status"] = store.DeviceApproved // with the confirmation off nothing waits
		}
	}
	return out
}

// loginDevice is the device part of a sign-in whose password is right: it finds or makes the device of the client. It returns the
// device to tie the session to (nil: the client has none and nothing is confirmed) and false when it has answered.
func (a *Admin) loginDevice(w http.ResponseWriter, r *http.Request, u *store.User) (*store.Device, bool) {
	since := a.gateSince(r.Context())
	tok, hasTok := deviceToken(r)
	if !hasTok {
		if since.IsZero() {
			return nil, true
		}
		a.audit(r, u, "auth.login", "user", "", u.Login, "device_required", nil)
		jsonErr(w, http.StatusForbidden, ErrDeviceRequired)
		return nil, false
	}
	status := store.DeviceApproved
	if !since.IsZero() {
		status = store.DevicePending
	}
	d, created, err := a.DB.RegisterDevice(r.Context(), u.ID, tok, clientInfo(r), clientIP(r), status)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if !since.IsZero() && d.Status == store.DeviceDenied {
		a.audit(r, u, "auth.login", "user", "", u.Login, "device_denied", map[string]any{"device": d.ID})
		jsonErr(w, http.StatusForbidden, ErrDeviceDenied)
		return nil, false
	}
	if created {
		if !since.IsZero() {
			a.audit(r, u, "device.request", "device", strconv.FormatInt(d.ID, 10), deviceName(d), "ok", map[string]any{"user": u.Login, "platform": d.Platform, "model": d.Model, "app": d.AppVersion, "ip": d.LastIP})
		}
		a.Live.PublishAdmin("devices", map[string]any{"id": d.ID})
	}
	return d, true
}

func deviceName(d *store.Device) string {
	switch {
	case d.Label != "":
		return d.Label
	case d.Model != "":
		return d.Model
	case d.Platform != "":
		return d.Platform
	}
	return "device " + strconv.FormatInt(d.ID, 10)
}

// ---- the API of Settings, "Devices" ----

type deviceJSON struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	User       string     `json:"user"`
	Platform   string     `json:"platform"`
	AppVersion string     `json:"app_version"`
	OSVersion  string     `json:"os_version"`
	Model      string     `json:"model"`
	Label      string     `json:"label"`
	Status     string     `json:"status"`
	LastIP     string     `json:"last_ip"`
	LastSeen   time.Time  `json:"last_seen"`
	CreatedAt  time.Time  `json:"created_at"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
	DecidedBy  string     `json:"decided_by,omitempty"`
	// Current: the device of this very request.
	Current bool `json:"current"`
}

type devicesState struct {
	Enabled bool         `json:"enabled"`
	Since   *time.Time   `json:"since,omitempty"`
	Pending int          `json:"pending"`
	Devices []deviceJSON `json:"devices"`
	// CanEnable: the application that asks has a device, which is approved when the confirmation is turned on (without it the
	// administrator would lock themselves out).
	CanEnable bool `json:"can_enable"`
}

func (a *Admin) devicesState(r *http.Request) (devicesState, error) {
	list, err := a.DB.ListDevices(r.Context())
	if err != nil {
		return devicesState{}, err
	}
	cur := deviceOfRequest(r)
	st := devicesState{Devices: make([]deviceJSON, 0, len(list)), CanEnable: cur != nil}
	if since := a.gateSince(r.Context()); !since.IsZero() {
		st.Enabled, st.Since = true, &since
	}
	for _, d := range list {
		if d.Status == store.DevicePending {
			st.Pending++
		}
		st.Devices = append(st.Devices, deviceJSON{ID: d.ID, UserID: d.UserID, User: d.Login, Platform: d.Platform, AppVersion: d.AppVersion, OSVersion: d.OSVersion, Model: d.Model,
			Label: d.Label, Status: d.Status, LastIP: d.LastIP, LastSeen: d.LastSeen, CreatedAt: d.CreatedAt, DecidedAt: d.DecidedAt, DecidedBy: d.DecidedBy,
			Current: cur != nil && cur.ID == d.ID})
	}
	return st, nil
}

func (a *Admin) devicesGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	st, err := a.devicesState(r)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type devicesGateReq struct {
	Enabled bool `json:"enabled"`
	// KeepSessions: the applications that are signed in now stay signed in (and are taken as approved). Without it everybody but the
	// one that asks is signed out and has to come through the confirmation.
	KeepSessions bool `json:"keep_sessions"`
	Confirm      bool `json:"confirm"`
}

func (a *Admin) devicesGate(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req devicesGateReq
	if !decode(w, r, &req) {
		return
	}
	if !req.Confirm {
		jsonErr(w, http.StatusConflict, "confirmation required")
		return
	}
	ctx := r.Context()
	was := !a.gateSince(ctx).IsZero()
	if req.Enabled {
		cur := deviceOfRequest(r)
		if cur == nil || cur.Status != store.DeviceApproved {
			// the application that asks must be a device that stays approved, or its administrator is locked out at once
			jsonErr(w, http.StatusBadRequest, ErrDeviceRequired)
			return
		}
		if !was {
			if err := a.DB.SetSetting(ctx, store.DeviceApprovalSetting, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				jsonErr(w, http.StatusInternalServerError, "internal error")
				return
			}
			if !req.KeepSessions {
				if se, ok := r.Context().Value(keySession).(*store.Session); ok {
					_ = a.DB.DeleteSessionsExcept(ctx, se.ID)
				}
			}
		}
	} else if was {
		if err := a.DB.SetSetting(ctx, store.DeviceApprovalSetting, ""); err != nil {
			jsonErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	a.gateReset()
	if req.Enabled != was {
		action := "device.gate.on"
		if !req.Enabled {
			action = "device.gate.off"
		}
		a.audit(r, u, action, "settings", "", "devices", "ok", map[string]any{"keep_sessions": req.KeepSessions})
		a.Live.PublishAdmin("devices", map[string]any{})
	}
	a.devicesGet(w, r, u)
}

func (a *Admin) deviceOf(w http.ResponseWriter, r *http.Request) (*store.Device, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "bad id")
		return nil, false
	}
	d, err := a.DB.DeviceByID(r.Context(), id)
	if err != nil {
		jsonErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return d, true
}

func (a *Admin) deviceDecide(status, action string) func(http.ResponseWriter, *http.Request, *store.User) {
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		d, ok := a.deviceOf(w, r)
		if !ok {
			return
		}
		if cur := deviceOfRequest(r); status == store.DeviceDenied && cur != nil && cur.ID == d.ID {
			jsonErr(w, http.StatusConflict, "this is the device you are using")
			return
		}
		if err := a.DB.SetDeviceStatus(r.Context(), d.ID, status, u.ID); err != nil {
			jsonErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		a.audit(r, u, action, "device", strconv.FormatInt(d.ID, 10), deviceName(d), "ok", map[string]any{"user": d.Login, "platform": d.Platform, "model": d.Model, "app": d.AppVersion, "ip": d.LastIP})
		a.Live.PublishAdmin("devices", map[string]any{"id": d.ID})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (a *Admin) deviceRemove(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req)
	if !req.Confirm {
		jsonErr(w, http.StatusConflict, "confirmation required")
		return
	}
	d, ok := a.deviceOf(w, r)
	if !ok {
		return
	}
	if err := a.DB.DeleteDevice(r.Context(), d.ID); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "device.remove", "device", strconv.FormatInt(d.ID, 10), deviceName(d), "ok", map[string]any{"user": d.Login, "platform": d.Platform, "model": d.Model, "ip": d.LastIP})
	a.Live.PublishAdmin("devices", map[string]any{"id": d.ID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Admin) deviceLabel(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &req) {
		return
	}
	d, ok := a.deviceOf(w, r)
	if !ok {
		return
	}
	label := strings.TrimSpace(req.Label)
	if len([]rune(label)) > 64 {
		jsonErr(w, http.StatusBadRequest, "name is too long")
		return
	}
	if err := a.DB.SetDeviceLabel(r.Context(), d.ID, label); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "device.label", "device", strconv.FormatInt(d.ID, 10), deviceName(d), "ok", map[string]any{"label": label})
	a.Live.PublishAdmin("devices", map[string]any{"id": d.ID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
