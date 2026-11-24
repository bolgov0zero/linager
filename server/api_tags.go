package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bolgov0zero/linager/server/store"
)

const (
	maxTags     = 20
	maxTagChars = 32
)

// cleanTags trims the tags, drops the empty ones and the repeats (case does not matter, the first spelling stays) and checks
// the limits; ok is false when a tag is too long, has a control character or there are too many.
func cleanTags(in []string) (out []string, ok bool) {
	seen := map[string]bool{}
	out = []string{}
	for _, t := range in {
		t = strings.Join(strings.Fields(t), " ")
		if t == "" {
			continue
		}
		if !utf8.ValidString(t) || utf8.RuneCountInString(t) > maxTagChars {
			return nil, false
		}
		for _, r := range t {
			if unicode.IsControl(r) {
				return nil, false
			}
		}
		k := strings.ToLower(t)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out, len(out) <= maxTags
}

// tagNames lists the names of the tags (the plain list the older callers read).
func tagNames(refs []store.TagRef) []string {
	out := make([]string, 0, len(refs))
	for _, t := range refs {
		out = append(out, t.Name)
	}
	return out
}

// serverTagsGet returns the tags of a server (any role that may see the server): the names, and the same tags with their
// ids and colours.
func (a *Admin) serverTagsGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	refs, err := a.DB.ServerTags(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tagNames(refs), "tag_items": refs})
}

// serverTagsSet replaces the tags of a server (administrators). A name the registry does not know is added to it. The tags
// decide who may see the server, so the audit entry says how many users this changed access for.
func (a *Admin) serverTagsSet(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request")
		return
	}
	tags, ok := cleanTags(req.Tags)
	if !ok {
		jsonErr(w, http.StatusUnprocessableEntity, "a tag is up to 32 characters without control characters, and a server has up to 20 tags")
		return
	}
	id := r.PathValue("id")
	refs, users, err := a.DB.SetServerTags(r.Context(), id, tags)
	switch {
	case errors.Is(err, store.ErrNotFound):
		jsonErr(w, http.StatusNotFound, "server not found")
		return
	case errors.Is(err, store.ErrTooManyTags):
		jsonErr(w, http.StatusUnprocessableEntity, "too many different tags")
		return
	case err != nil:
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	name := id
	if s, err := a.DB.ServerByID(r.Context(), id); err == nil {
		name = s.Name
	}
	a.audit(r, u, "server.tags", "server", id, name, "ok", map[string]any{"tags": tagNames(refs), "access_users": users})
	if users > 0 {
		a.Live.Publish("dashboard", map[string]any{}) // the lists of those users changed
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tagNames(refs), "tag_items": refs})
}

// tagsList returns the registry with the number of servers and users of each tag (administrators).
func (a *Admin) tagsList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := a.DB.Tags(r.Context())
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": list, "colors": store.TagColors, "max": store.MaxTagsTotal})
}

// cleanTagName checks one tag name by the same rules as the names a server gets.
func cleanTagName(s string) (string, bool) {
	out, ok := cleanTags([]string{s})
	if !ok || len(out) != 1 {
		return "", false
	}
	return out[0], true
}

func (a *Admin) tagCreate(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decode(w, r, &req) {
		return
	}
	name, ok := cleanTagName(req.Name)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "invalid tag name")
		return
	}
	if req.Color == "" {
		req.Color = store.DefaultTagColor
	}
	if !store.ValidTagColor(req.Color) {
		jsonErr(w, http.StatusBadRequest, "unknown colour")
		return
	}
	t, err := a.DB.TagCreate(r.Context(), name, req.Color)
	switch {
	case errors.Is(err, store.ErrTagExists):
		jsonErr(w, http.StatusConflict, "tag name already in use")
		return
	case errors.Is(err, store.ErrTooManyTags):
		jsonErr(w, http.StatusUnprocessableEntity, "too many different tags")
		return
	case err != nil:
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "tag.create", "tag", strconv.FormatInt(t.ID, 10), t.Name, "ok", map[string]any{"color": t.Color})
	writeJSON(w, http.StatusOK, t)
}

// tagUpdate renames a tag and/or changes its colour. Everything that refers to the tag (servers, grants) keeps working.
func (a *Admin) tagUpdate(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request")
		return
	}
	var req struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Name == nil && req.Color == nil {
		jsonErr(w, http.StatusBadRequest, "nothing to change")
		return
	}
	if req.Name != nil {
		n, ok := cleanTagName(*req.Name)
		if !ok {
			jsonErr(w, http.StatusBadRequest, "invalid tag name")
			return
		}
		req.Name = &n
	}
	if req.Color != nil && !store.ValidTagColor(*req.Color) {
		jsonErr(w, http.StatusBadRequest, "unknown colour")
		return
	}
	old, err := a.DB.TagUpdate(r.Context(), id, req.Name, req.Color)
	switch {
	case errors.Is(err, store.ErrNotFound):
		jsonErr(w, http.StatusNotFound, "tag not found")
		return
	case errors.Is(err, store.ErrTagExists):
		jsonErr(w, http.StatusConflict, "tag name already in use")
		return
	case err != nil:
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	sid := strconv.FormatInt(id, 10)
	if req.Name != nil && *req.Name != old.Name {
		a.audit(r, u, "tag.rename", "tag", sid, *req.Name, "ok", map[string]any{"was": old.Name})
	}
	if req.Color != nil && *req.Color != old.Color {
		name := old.Name
		if req.Name != nil {
			name = *req.Name
		}
		a.audit(r, u, "tag.color", "tag", sid, name, "ok", map[string]any{"color": *req.Color, "was": old.Color})
	}
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// tagDelete removes a tag from every server and from every grant; the answer and the audit entry say how many there were.
func (a *Admin) tagDelete(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "bad request")
		return
	}
	t, err := a.DB.TagDelete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		jsonErr(w, http.StatusNotFound, "tag not found")
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "tag.delete", "tag", strconv.FormatInt(id, 10), t.Name, "ok", map[string]any{"servers": t.Servers, "users": t.Users})
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "servers": t.Servers, "users": t.Users})
}
