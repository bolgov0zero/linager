package server_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type tagRow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Servers int    `json:"servers"`
	Users   int    `json:"users"`
}

type tagList struct {
	Tags   []tagRow `json:"tags"`
	Colors []string `json:"colors"`
}

// TestTagRegistry: tags are a registry. Typing a name on a server adds it (case does not matter), the registry can create,
// rename and recolour a tag, a rename keeps everything that refers to the tag, and a delete takes it off every server.
func TestTagRegistry(t *testing.T) {
	base, db := startAccountsServer(t)
	ctx := context.Background()
	add := func(name, addr string) string {
		var id string
		if err := db.Pool.QueryRow(ctx, `INSERT INTO servers (name,hostname,address,agent_pubkey,os,agent_version,protocol)
			VALUES ($1,$1,$2,convert_to($1,'UTF8'),'Debian','0.1.0',1) RETURNING id::text`, name, addr).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	s1, s2 := add("web", "10.0.0.1"), add("db", "10.0.0.2")
	admin := mustLogin(t, base, "admin", "a-long-test-password")

	var got struct {
		Tags  []string `json:"tags"`
		Items []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"tag_items"`
	}
	if code := admin.do("PUT", "/api/servers/"+s1+"/tags", map[string]any{"tags": []string{"prod", "Web"}}, &got); code != 200 || strings.Join(got.Tags, "|") != "prod|Web" || len(got.Items) != 2 || got.Items[0].Color != "gray" {
		t.Fatalf("set: %d %+v", code, got)
	}
	// the same tag in another case is the same tag: the registry spelling wins
	got.Tags = nil
	if code := admin.do("PUT", "/api/servers/"+s2+"/tags", map[string]any{"tags": []string{"PROD"}}, &got); code != 200 || strings.Join(got.Tags, "|") != "prod" {
		t.Fatalf("same tag, other case: %d %+v", code, got)
	}
	var reg tagList
	if code := admin.do("GET", "/api/tags", nil, &reg); code != 200 || len(reg.Tags) != 2 || len(reg.Colors) < 16 {
		t.Fatalf("registry: %d %+v", code, reg)
	}
	var prod, web tagRow
	for _, x := range reg.Tags {
		if x.Name == "prod" {
			prod = x
		} else {
			web = x
		}
	}
	if prod.Servers != 2 || web.Servers != 1 {
		t.Fatalf("counts: %+v %+v", prod, web)
	}

	// create: a name that is taken (other case), a bad colour, an empty name
	var made tagRow
	if code := admin.do("POST", "/api/tags", map[string]any{"name": "  Staging ", "color": "blue"}, &made); code != 200 || made.Name != "Staging" || made.Color != "blue" {
		t.Fatalf("create: %d %+v", code, made)
	}
	if code := admin.do("POST", "/api/tags", map[string]any{"name": "STAGING"}, nil); code != http.StatusConflict {
		t.Fatalf("duplicate name: %d", code)
	}
	if code := admin.do("POST", "/api/tags", map[string]any{"name": "x", "color": "purple"}, nil); code != http.StatusBadRequest {
		t.Fatalf("colour outside the palette: %d", code)
	}
	if code := admin.do("POST", "/api/tags", map[string]any{"name": "   "}, nil); code != http.StatusBadRequest {
		t.Fatalf("empty name: %d", code)
	}

	// rename and recolour: the servers show the new name, a taken name is refused, an unknown tag is 404
	pid := strconv.FormatInt(prod.ID, 10)
	if code := admin.do("PUT", "/api/tags/"+pid, map[string]any{"name": "production", "color": "red"}, nil); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	got.Tags, got.Items = nil, nil
	if code := admin.do("GET", "/api/servers/"+s2+"/tags", nil, &got); code != 200 || strings.Join(got.Tags, "|") != "production" || got.Items[0].Color != "red" {
		t.Fatalf("the server shows the new name and colour: %d %+v", code, got)
	}
	if code := admin.do("PUT", "/api/tags/"+pid, map[string]any{"name": "WEB"}, nil); code != http.StatusConflict {
		t.Fatalf("rename onto a taken name: %d", code)
	}
	if code := admin.do("PUT", "/api/tags/"+pid, map[string]any{"color": "violet"}, nil); code != http.StatusBadRequest {
		t.Fatalf("bad colour: %d", code)
	}
	if code := admin.do("PUT", "/api/tags/"+pid, map[string]any{}, nil); code != http.StatusBadRequest {
		t.Fatalf("nothing to change: %d", code)
	}
	if code := admin.do("PUT", "/api/tags/999999", map[string]any{"name": "ghost"}, nil); code != http.StatusNotFound {
		t.Fatalf("unknown tag: %d", code)
	}
	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action IN ('tag.create','tag.rename','tag.color')`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("audit entries of the registry: %d %v", n, err)
	}

	// delete: it goes off every server, the answer says how many there were
	var del struct {
		Servers int `json:"servers"`
		Users   int `json:"users"`
	}
	if code := admin.do("DELETE", "/api/tags/"+pid, nil, &del); code != 200 || del.Servers != 2 {
		t.Fatalf("delete: %d %+v", code, del)
	}
	got.Tags = nil
	if code := admin.do("GET", "/api/servers/"+s1+"/tags", nil, &got); code != 200 || strings.Join(got.Tags, "|") != "Web" {
		t.Fatalf("the deleted tag is gone from the server: %d %+v", code, got)
	}
	if code := admin.do("DELETE", "/api/tags/"+pid, nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", code)
	}

	// the other roles cannot touch the registry (and cannot even read it)
	if code := admin.do("POST", "/api/users", map[string]any{"login": "oper", "password": "op-long-password-1", "role": "operator", "servers": []string{s1}, "modules": []string{}}, nil); code != 200 {
		t.Fatalf("create operator: %d", code)
	}
	op := mustLogin(t, base, "oper", "op-long-password-1")
	for _, c := range []struct{ method, path string }{{"GET", "/api/tags"}, {"POST", "/api/tags"}, {"PUT", "/api/tags/" + strconv.FormatInt(web.ID, 10)}, {"DELETE", "/api/tags/" + strconv.FormatInt(web.ID, 10)}} {
		if code := op.do(c.method, c.path, map[string]any{"name": "x"}, nil); code != http.StatusForbidden {
			t.Fatalf("an operator must not %s %s: %d", c.method, c.path, code)
		}
	}
}

// TestTagGrants: a user can be given a tag next to servers and groups, sees every server with the tag, keeps it through a
// rename, and loses it when the tag is taken off the server or deleted. The audit says whom a tag change reached.
func TestTagGrants(t *testing.T) {
	base, db := startAccountsServer(t)
	ctx := context.Background()
	add := func(name, addr string) string {
		var id string
		if err := db.Pool.QueryRow(ctx, `INSERT INTO servers (name,hostname,address,agent_pubkey,os,agent_version,protocol)
			VALUES ($1,$1,$2,convert_to($1,'UTF8'),'Debian','0.1.0',1) RETURNING id::text`, name, addr).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b, c, d := add("a", "10.0.0.1"), add("b", "10.0.0.2"), add("c", "10.0.0.3"), add("d", "10.0.0.4")
	admin := mustLogin(t, base, "admin", "a-long-test-password")
	var prod tagRow
	if code := admin.do("POST", "/api/tags", map[string]any{"name": "prod", "color": "green"}, &prod); code != 200 {
		t.Fatalf("create tag: %d", code)
	}
	for _, id := range []string{a, b} {
		if code := admin.do("PUT", "/api/servers/"+id+"/tags", map[string]any{"tags": []string{"prod"}}, nil); code != 200 {
			t.Fatalf("tag %s: %d", id, code)
		}
	}
	// operator: the tag + one more single server; auditor: the tag only
	if code := admin.do("POST", "/api/users", map[string]any{"login": "oper", "password": "op-long-password-1", "role": "operator", "servers": []string{c}, "tags": []int64{prod.ID}, "modules": []string{}}, nil); code != 200 {
		t.Fatalf("create operator: %d", code)
	}
	if code := admin.do("POST", "/api/users", map[string]any{"login": "audi", "password": "au-long-password-1", "role": "auditor", "tags": []int64{prod.ID, prod.ID}}, nil); code != 200 {
		t.Fatalf("create auditor (a tag given twice is fine): %d", code)
	}
	op := mustLogin(t, base, "oper", "op-long-password-1")
	au := mustLogin(t, base, "audi", "au-long-password-1")
	visible := func(cl *apiClient, id string) bool { return cl.do("GET", "/api/servers/"+id, nil, nil) == 200 }
	check := func(label string, cl *apiClient, want map[string]bool) {
		t.Helper()
		for id, w := range want {
			if visible(cl, id) != w {
				t.Fatalf("%s: server %s visible=%v, want %v", label, id, !w, w)
			}
		}
	}
	check("operator", op, map[string]bool{a: true, b: true, c: true, d: false})
	check("auditor", au, map[string]bool{a: true, b: true, c: false, d: false})

	// the access of the user comes back with the tag
	var users struct {
		Users []struct {
			Login  string `json:"login"`
			Access *struct {
				Tags    []int64  `json:"tags"`
				Servers []string `json:"servers"`
			} `json:"access"`
		} `json:"users"`
	}
	if code := admin.do("GET", "/api/users", nil, &users); code != 200 {
		t.Fatalf("users: %d", code)
	}
	for _, u := range users.Users {
		if u.Login == "oper" && (u.Access == nil || len(u.Access.Tags) != 1 || u.Access.Tags[0] != prod.ID) {
			t.Fatalf("the operator's tags: %+v", u.Access)
		}
	}

	// a rename keeps the access
	pid := strconv.FormatInt(prod.ID, 10)
	if code := admin.do("PUT", "/api/tags/"+pid, map[string]any{"name": "production"}, nil); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	check("operator after the rename", op, map[string]bool{a: true, b: true, c: true, d: false})

	// a tag put on a server gives it, a tag taken off removes it; the audit counts the users
	if code := admin.do("PUT", "/api/servers/"+d+"/tags", map[string]any{"tags": []string{"production"}}, nil); code != 200 {
		t.Fatalf("tag d: %d", code)
	}
	check("tag put on d", op, map[string]bool{d: true})
	check("tag put on d (auditor)", au, map[string]bool{d: true})
	if code := admin.do("PUT", "/api/servers/"+d+"/tags", map[string]any{"tags": []string{}}, nil); code != 200 {
		t.Fatalf("untag d: %d", code)
	}
	check("tag taken off d", op, map[string]bool{d: false})
	var users2 int
	if err := db.Pool.QueryRow(ctx, `SELECT (details->>'access_users')::int FROM audit_log WHERE action='server.tags' AND target_id=$1 ORDER BY id DESC LIMIT 1`, d).Scan(&users2); err != nil || users2 != 2 {
		t.Fatalf("the audit entry must say that 2 users were reached: %d %v", users2, err)
	}

	// a tag that does not exist cannot be given
	if code := admin.do("PUT", "/api/users/"+userID(t, admin, "oper")+"/access", map[string]any{"role": "operator", "tags": []int64{424242}, "modules": []string{}}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown tag: %d", code)
	}
	// the failed change left the grant as it was
	check("operator after the refused change", op, map[string]bool{a: true, c: true})

	// the delete of the tag says whom it reached and takes the access with it
	var del struct {
		Servers int `json:"servers"`
		Users   int `json:"users"`
	}
	if code := admin.do("DELETE", "/api/tags/"+pid, nil, &del); code != 200 || del.Servers != 2 || del.Users != 2 {
		t.Fatalf("delete: %d %+v", code, del)
	}
	check("operator after the delete", op, map[string]bool{a: false, b: false, c: true})
	check("auditor after the delete", au, map[string]bool{a: false, b: false, c: false})
	var left int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM user_tags`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("the grants of a deleted tag must go: %d %v", left, err)
	}
}

// userID returns the id of a user by login, as the text of a path.
func userID(t *testing.T, cl *apiClient, login string) string {
	t.Helper()
	var list struct {
		Users []struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"users"`
	}
	if code := cl.do("GET", "/api/users", nil, &list); code != 200 {
		t.Fatalf("users: %d", code)
	}
	for _, u := range list.Users {
		if u.Login == login {
			return strconv.FormatInt(u.ID, 10)
		}
	}
	t.Fatalf("no user %s", login)
	return ""
}
