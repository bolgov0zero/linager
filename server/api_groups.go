package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bolgov0zero/linager/server/store"
)

const maxGroupName = 40

// cleanGroupName trims and squeezes the spaces of a folder name and checks it: 1 to 40 characters, no control characters.
func cleanGroupName(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxGroupName {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}

// groupFail answers a failed change of the folders.
func groupFail(w http.ResponseWriter, err error) {
	var clash *store.NameClash
	switch {
	case errors.As(err, &clash):
		jsonErr(w, http.StatusConflict, "a folder named \""+clash.Name+"\" already exists here")
	case errors.Is(err, store.ErrGroupExists):
		jsonErr(w, http.StatusConflict, "folder name already in use here")
	case errors.Is(err, store.ErrNotFound):
		jsonErr(w, http.StatusNotFound, "folder not found")
	case errors.Is(err, store.ErrGroupCycle):
		jsonErr(w, http.StatusUnprocessableEntity, "a folder cannot be put into itself or into its own subfolder")
	case errors.Is(err, store.ErrGroupDepth):
		jsonErr(w, http.StatusUnprocessableEntity, "too many levels of folders (up to "+strconv.Itoa(store.MaxGroupDepth)+")")
	case errors.Is(err, store.ErrTooManyGroups):
		jsonErr(w, http.StatusUnprocessableEntity, "too many folders")
	case errors.Is(err, store.ErrGroupBefore):
		jsonErr(w, http.StatusBadRequest, "the folder to put before is not in this folder")
	default:
		jsonErr(w, http.StatusInternalServerError, "internal error")
	}
}

// groupsList returns every folder with its parent, in the order of the administrator (the order inside a parent is the order
// of the list). Any role may read it: the names are needed to show the dashboard.
func (a *Admin) groupsList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := a.DB.Groups(r.Context())
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *Admin) groupCreate(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	name, ok := cleanGroupName(req.Name)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "invalid folder name")
		return
	}
	g, err := a.DB.GroupCreate(r.Context(), name, req.ParentID)
	if err != nil {
		groupFail(w, err)
		return
	}
	path, _ := a.DB.GroupPath(r.Context(), g.ID)
	a.audit(r, u, "group.create", "group", strconv.FormatInt(g.ID, 10), g.Name, "ok", map[string]any{"path": path})
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, g)
}

func (a *Admin) groupRename(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Name string `json:"name"`
	}
	if err != nil || !decode(w, r, &req) {
		if err != nil {
			jsonErr(w, http.StatusBadRequest, "bad request")
		}
		return
	}
	name, ok := cleanGroupName(req.Name)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "invalid folder name")
		return
	}
	was, _ := a.DB.GroupName(r.Context(), id)
	if err := a.DB.GroupRename(r.Context(), id, name); err != nil {
		groupFail(w, err)
		return
	}
	a.audit(r, u, "group.rename", "group", strconv.FormatInt(id, 10), name, "ok", map[string]any{"was": was})
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// groupMove puts a folder under another one (or on the top level) before a sibling (or last). With dry_run it only says what the
// move would do to the access of the users: the folders carry access, so a move can give or take servers.
func (a *Admin) groupMove(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		ParentID *int64 `json:"parent_id"`
		BeforeID *int64 `json:"before_id"`
		DryRun   bool   `json:"dry_run"`
	}
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if !decode(w, r, &req) {
		return
	}
	was, _ := a.DB.GroupPath(r.Context(), id)
	imp, err := a.DB.GroupMove(r.Context(), id, req.ParentID, req.BeforeID, req.DryRun)
	if err != nil {
		groupFail(w, err)
		return
	}
	if !req.DryRun {
		now, _ := a.DB.GroupPath(r.Context(), id)
		name, _ := a.DB.GroupName(r.Context(), id)
		a.audit(r, u, "group.move", "group", strconv.FormatInt(id, 10), name, "ok", map[string]any{"was": was, "path": now, "access_users": len(imp)})
		a.Live.Publish("dashboard", map[string]any{})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dry_run": req.DryRun, "impact": imp})
}

// groupDelete removes a folder: what is in it moves up one level. ?dry_run=1 only says what would happen.
func (a *Admin) groupDelete(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request")
		return
	}
	dry := r.URL.Query().Get("dry_run") == "1"
	res, err := a.DB.GroupDelete(r.Context(), id, dry)
	if err != nil {
		groupFail(w, err)
		return
	}
	if !dry {
		a.audit(r, u, "group.delete", "group", strconv.FormatInt(id, 10), res.Name, "ok",
			map[string]any{"subfolders": res.Subfolders, "servers": res.Servers, "grants": res.Grants, "access_users": len(res.Impact)})
		a.Live.Publish("dashboard", map[string]any{})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dry_run": dry, "name": res.Name, "subfolders": res.Subfolders, "servers": res.Servers, "grants": res.Grants, "impact": res.Impact})
}

// serverSetGroup puts a server into a folder; group_id null removes it from its folder. With dry_run it only says what that
// would do to the access of the users.
func (a *Admin) serverSetGroup(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		GroupID *int64 `json:"group_id"`
		DryRun  bool   `json:"dry_run"`
	}
	if !decode(w, r, &req) {
		return
	}
	srv, err := a.DB.ServerByID(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, http.StatusNotFound, "server not found")
		return
	}
	path := ""
	if req.GroupID != nil {
		if path, err = a.DB.GroupPath(r.Context(), *req.GroupID); err != nil {
			jsonErr(w, http.StatusNotFound, "folder not found")
			return
		}
	}
	imp, err := a.DB.MoveServerToGroup(r.Context(), srv.ID, req.GroupID, req.DryRun)
	if err != nil {
		groupFail(w, err)
		return
	}
	if !req.DryRun {
		a.audit(r, u, "group.assign", "server", srv.ID, srv.Name, "ok", map[string]any{"group": path, "access_users": len(imp)})
		a.Live.Publish("dashboard", map[string]any{})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dry_run": req.DryRun, "impact": imp})
}
