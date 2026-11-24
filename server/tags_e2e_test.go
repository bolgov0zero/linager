package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestServerTags: an administrator sets the tags of a server (trimmed, without repeats, limited), they come back with the
// server and in the list, are written to the audit, and the other roles can read them but not change them.
func TestServerTags(t *testing.T) {
	base, db := startAccountsServer(t)
	ctx := context.Background()
	var id string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO servers (name,hostname,address,agent_pubkey,os,agent_version,protocol)
		VALUES ('web','web','10.0.0.1','\x00','Debian','0.1.0',1) RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	admin := mustLogin(t, base, "admin", "a-long-test-password")

	var got struct {
		Tags []string `json:"tags"`
	}
	if code := admin.do("GET", "/api/servers/"+id, nil, &got); code != 200 || got.Tags == nil || len(got.Tags) != 0 {
		t.Fatalf("a new server has no tags (an empty list, not null): %d %+v", code, got)
	}
	in := map[string]any{"tags": []string{"  prod ", "Web  tier", "PROD", "", "db"}}
	if code := admin.do("PUT", "/api/servers/"+id+"/tags", in, &got); code != 200 || strings.Join(got.Tags, "|") != "prod|Web tier|db" {
		t.Fatalf("save: %d %+v", code, got)
	}
	got.Tags = nil
	if code := admin.do("GET", "/api/servers/"+id+"/tags", nil, &got); code != 200 || strings.Join(got.Tags, "|") != "prod|Web tier|db" {
		t.Fatalf("list: %d %+v", code, got)
	}
	got.Tags = nil
	if code := admin.do("GET", "/api/servers/"+id, nil, &got); code != 200 || strings.Join(got.Tags, "|") != "prod|Web tier|db" {
		t.Fatalf("the tags come back with the server: %d %+v", code, got)
	}
	// a smaller set replaces the bigger one
	if code := admin.do("PUT", "/api/servers/"+id+"/tags", map[string]any{"tags": []string{"db"}}, &got); code != 200 || len(got.Tags) != 1 {
		t.Fatalf("replace: %d %+v", code, got)
	}
	// limits
	if code := admin.do("PUT", "/api/servers/"+id+"/tags", map[string]any{"tags": []string{strings.Repeat("я", 33)}}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("a long tag: %d", code)
	}
	if code := admin.do("PUT", "/api/servers/"+id+"/tags", map[string]any{"tags": []string{"a\tb\x01"}}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("a control character: %d", code)
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = "t" + strings.Repeat("x", i)
	}
	if code := admin.do("PUT", "/api/servers/"+id+"/tags", map[string]any{"tags": many}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("21 tags: %d", code)
	}
	if code := admin.do("PUT", "/api/servers/00000000-0000-0000-0000-000000000000/tags", map[string]any{"tags": []string{"x"}}, nil); code != http.StatusNotFound {
		t.Fatalf("an unknown server: %d", code)
	}
	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action='server.tags' AND target_id=$1`, id).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit entries: %d %v", n, err)
	}

	// an operator and an auditor read the tags with the server but cannot change them
	for _, role := range []string{"operator", "auditor"} {
		if code := admin.do("POST", "/api/users", map[string]any{"login": role, "password": role + "-long-password", "role": role, "servers": []string{id}, "modules": []string{}}, nil); code != 200 {
			t.Fatalf("create %s: %d", role, code)
		}
		c := mustLogin(t, base, role, role+"-long-password")
		if code := c.do("PUT", "/api/servers/"+id+"/tags", map[string]any{"tags": []string{"mine"}}, nil); code != http.StatusForbidden {
			t.Fatalf("%s must not change the tags: %d", role, code)
		}
		got.Tags = nil
		if code := c.do("GET", "/api/servers/"+id+"/tags", nil, &got); code != 200 || len(got.Tags) != 1 {
			t.Fatalf("%s must read the tags: %d %+v", role, code, got)
		}
	}
	// the tags go with the server
	if code := admin.do("DELETE", "/api/servers/"+id, map[string]any{"name": "web", "uninstall": false}, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM server_tags`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the tags of a deleted server must go too: %d %v", n, err)
	}
}
