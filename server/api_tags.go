package server

import (
	"encoding/json"
	"net/http"
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

// serverTagsGet returns the tags of a server (any role that may see the server).
func (a *Admin) serverTagsGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	tags, err := a.DB.ServerTags(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// serverTagsSet replaces the tags of a server (administrators).
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
	if err := a.DB.SetServerTags(r.Context(), id, tags); err != nil {
		jsonErr(w, http.StatusNotFound, "server not found")
		return
	}
	name := id
	if s, err := a.DB.ServerByID(r.Context(), id); err == nil {
		name = s.Name
	}
	a.audit(r, u, "server.tags", "server", id, name, "ok", map[string]any{"tags": tags})
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}
