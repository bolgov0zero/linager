package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/bolgov0zero/linager/server/store"
)

// Settings, "Connection": the port of the agents and the port of the desktop application.

func (a *Admin) connWrite(w http.ResponseWriter, err error) {
	var ce *ConnError
	if errors.As(err, &ce) {
		body := map[string]any{"error": ce.Code, "detail": ce.Detail}
		if len(ce.Blockers) > 0 {
			body["agents"] = ce.Blockers
		}
		writeJSON(w, ce.Status, body)
		return
	}
	a.Log.Error("connection settings", "err", err)
	jsonErr(w, http.StatusInternalServerError, "internal error")
}

func (a *Admin) connStatus(w http.ResponseWriter, r *http.Request) {
	st, err := a.Conn.Status(r.Context())
	if err != nil {
		a.connWrite(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (a *Admin) connGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if a.Conn == nil {
		jsonErr(w, http.StatusNotFound, "not available")
		return
	}
	a.connStatus(w, r)
}

func (a *Admin) connPut(w http.ResponseWriter, r *http.Request, u *store.User) {
	if a.Conn == nil {
		jsonErr(w, http.StatusNotFound, "not available")
		return
	}
	var req ConnReq
	if !decode(w, r, &req) {
		return
	}
	before := a.Conn.State()
	if err := a.Conn.Apply(r.Context(), req); err != nil {
		a.audit(r, u, "connection.update", "connection", "", strconv.Itoa(req.AgentPort), "failed", map[string]any{"agent_port": req.AgentPort, "app_port": req.AppPort, "error": err.Error()})
		a.connWrite(w, err)
		return
	}
	a.audit(r, u, "connection.update", "connection", "", strconv.Itoa(req.AgentPort), "ok", map[string]any{
		"agent_port": req.AgentPort, "app_port": req.AppPort, "was_agent_port": before.AgentPort, "was_app_port": before.AppPort})
	a.connStatus(w, r)
}

func (a *Admin) connCloseOld(w http.ResponseWriter, r *http.Request, u *store.User) {
	if a.Conn == nil {
		jsonErr(w, http.StatusNotFound, "not available")
		return
	}
	old := a.Conn.State().LegacyPort
	if err := a.Conn.CloseLegacy(r.Context()); err != nil {
		a.connWrite(w, err)
		return
	}
	a.audit(r, u, "connection.close_old", "connection", "", strconv.Itoa(old), "ok", map[string]any{"port": old})
	a.connStatus(w, r)
}
