package server_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
)

type folderRow struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID *int64 `json:"parent_id"`
}

type impactRow struct {
	Login string   `json:"login"`
	Lose  []string `json:"lose"`
	Gain  []string `json:"gain"`
}

// TestNestedFolders: folders nest, the name is unique only inside a parent, a folder cannot go into itself or too deep, a move
// and a delete keep the order and report what they do to the access of the users, and an access given on a folder covers
// everything under it, also what comes later.
func TestNestedFolders(t *testing.T) {
	base, db := startAccountsServer(t)
	ctx := context.Background()
	addServer := func(name, addr string) string {
		var id string
		if err := db.Pool.QueryRow(ctx, `INSERT INTO servers (name,hostname,address,agent_pubkey,os,agent_version,protocol)
			VALUES ($1,$1,$2,convert_to($1,'UTF8'),'Debian','0.1.0',1) RETURNING id::text`, name, addr).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	admin := mustLogin(t, base, "admin", "a-long-test-password")
	gid := func(id int64) string { return strconv.FormatInt(id, 10) }
	mk := func(name string, parent *int64) folderRow {
		t.Helper()
		var f folderRow
		if code := admin.do("POST", "/api/groups", map[string]any{"name": name, "parent_id": parent}, &f); code != 200 {
			t.Fatalf("create %q: %d", name, code)
		}
		return f
	}
	list := func() []folderRow {
		t.Helper()
		var l []folderRow
		if code := admin.do("GET", "/api/groups", nil, &l); code != 200 {
			t.Fatalf("list: %d", code)
		}
		return l
	}
	// kids names in order, for a parent (nil = the top level)
	kids := func(parent *int64) string {
		out := ""
		for _, f := range list() {
			if (f.ParentID == nil && parent == nil) || (f.ParentID != nil && parent != nil && *f.ParentID == *parent) {
				out += f.Name + ","
			}
		}
		return out
	}

	prod := mk("Prod", nil)
	web := mk("Web", &prod.ID)
	front := mk("Front", &web.ID)
	test := mk("Test", nil)
	if list()[2].ParentID == nil || *list()[2].ParentID != web.ID {
		t.Fatalf("the parent comes back in the list: %+v", list())
	}

	// the name is unique inside a parent only
	mk("Web", &test.ID) // the same name under another parent is fine
	if code := admin.do("POST", "/api/groups", map[string]any{"name": " WEB ", "parent_id": prod.ID}, nil); code != http.StatusConflict {
		t.Fatalf("the same name in the same parent: %d", code)
	}
	if code := admin.do("POST", "/api/groups", map[string]any{"name": "prod"}, nil); code != http.StatusConflict {
		t.Fatalf("the same name on the top level: %d", code)
	}
	if code := admin.do("POST", "/api/groups", map[string]any{"name": "x", "parent_id": 424242}, nil); code != http.StatusNotFound {
		t.Fatalf("an unknown parent: %d", code)
	}
	if code := admin.do("PUT", "/api/groups/"+gid(test.ID), map[string]any{"name": "prod"}, nil); code != http.StatusConflict {
		t.Fatalf("a rename onto a sibling: %d", code)
	}

	// a folder cannot go into itself or into its own subfolder
	for _, target := range []int64{prod.ID, web.ID, front.ID} {
		if code := admin.do("PUT", "/api/groups/"+gid(prod.ID)+"/move", map[string]any{"parent_id": target}, nil); code != http.StatusUnprocessableEntity {
			t.Fatalf("Prod into %d must be refused: %d", target, code)
		}
	}
	// the depth is limited to 8 levels: Prod/Web/Front is 3, five more fit, the next does not
	parent := front.ID
	for i := 0; i < 5; i++ {
		parent = mk("L"+strconv.Itoa(i), &parent).ID
	}
	if code := admin.do("POST", "/api/groups", map[string]any{"name": "L-deep", "parent_id": parent}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("the 9th level: %d", code)
	}
	// moving a deep subtree under a deeper parent is refused too
	if code := admin.do("PUT", "/api/groups/"+gid(prod.ID)+"/move", map[string]any{"parent_id": test.ID}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("Prod (8 levels) under Test would be 9: %d", code)
	}

	// the order: a folder goes before a sibling, or last
	ops := mk("Ops", nil)
	if code := admin.do("PUT", "/api/groups/"+gid(ops.ID)+"/move", map[string]any{"parent_id": nil, "before_id": prod.ID}, nil); code != 200 {
		t.Fatalf("move before: %d", code)
	}
	if got := kids(nil); got != "Ops,Prod,Test," {
		t.Fatalf("order after the move: %s", got)
	}
	if code := admin.do("PUT", "/api/groups/"+gid(ops.ID)+"/move", map[string]any{"parent_id": nil}, nil); code != 200 {
		t.Fatalf("move last: %d", code)
	}
	if got := kids(nil); got != "Prod,Test,Ops," {
		t.Fatalf("order after the move to the end: %s", got)
	}
	// a place before a folder of another parent is refused
	if code := admin.do("PUT", "/api/groups/"+gid(ops.ID)+"/move", map[string]any{"parent_id": prod.ID, "before_id": test.ID}, nil); code != http.StatusBadRequest {
		t.Fatalf("before a folder of another parent: %d", code)
	}

	// access follows the tree: the operator is given Prod and sees the servers of Web and Front, also a new one
	sWeb, sFront, sTest := addServer("web-1", "10.0.0.1"), addServer("front-1", "10.0.0.2"), addServer("test-1", "10.0.0.3")
	for id, g := range map[string]int64{sWeb: web.ID, sFront: front.ID, sTest: test.ID} {
		if code := admin.do("PUT", "/api/servers/"+id+"/group", map[string]any{"group_id": g}, nil); code != 200 {
			t.Fatalf("set group: %d", code)
		}
	}
	if code := admin.do("POST", "/api/users", map[string]any{"login": "oper", "password": "op-long-password-1", "role": "operator", "groups": []int64{prod.ID}, "modules": []string{}}, nil); code != 200 {
		t.Fatalf("create operator: %d", code)
	}
	op := mustLogin(t, base, "oper", "op-long-password-1")
	sees := func(id string) bool { return op.do("GET", "/api/servers/"+id, nil, nil) == 200 }
	if !sees(sWeb) || !sees(sFront) || sees(sTest) {
		t.Fatalf("the operator sees what is under Prod and nothing else: %v %v %v", sees(sWeb), sees(sFront), sees(sTest))
	}
	sNew := addServer("new-1", "10.0.0.4")
	if code := admin.do("PUT", "/api/servers/"+sNew+"/group", map[string]any{"group_id": front.ID}, nil); code != 200 || !sees(sNew) {
		t.Fatalf("a server put under Prod later is visible at once: %d", code)
	}

	// the preview of a move names who gains and who loses, and changes nothing
	var pv struct {
		DryRun bool        `json:"dry_run"`
		Impact []impactRow `json:"impact"`
	}
	if code := admin.do("PUT", "/api/groups/"+gid(web.ID)+"/move", map[string]any{"parent_id": test.ID, "dry_run": true}, &pv); code != http.StatusConflict {
		t.Fatalf("Web into Test meets the Web that is there: %d", code)
	}
	if code := admin.do("PUT", "/api/groups/"+gid(front.ID)+"/move", map[string]any{"parent_id": test.ID, "dry_run": true}, &pv); code != 200 || !pv.DryRun || len(pv.Impact) != 1 || pv.Impact[0].Login != "oper" || len(pv.Impact[0].Lose) != 2 || len(pv.Impact[0].Gain) != 0 {
		t.Fatalf("preview: %d %+v", code, pv)
	}
	if !sees(sFront) {
		t.Fatalf("a preview must not move anything")
	}
	// the real move: the operator loses Front, the audit says how many users it reached
	if code := admin.do("PUT", "/api/groups/"+gid(front.ID)+"/move", map[string]any{"parent_id": test.ID}, nil); code != 200 {
		t.Fatalf("move Front: %d", code)
	}
	if sees(sFront) || sees(sNew) || !sees(sWeb) {
		t.Fatalf("the operator lost Front and what was in it: %v %v %v", sees(sFront), sees(sNew), sees(sWeb))
	}
	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT (details->>'access_users')::int FROM audit_log WHERE action='group.move' ORDER BY id DESC LIMIT 1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the audit of a move says whom it reached: %d %v", n, err)
	}
	// a server moved by hand: the preview and the audit too
	if code := admin.do("PUT", "/api/servers/"+sWeb+"/group", map[string]any{"group_id": test.ID, "dry_run": true}, &pv); code != 200 || len(pv.Impact) != 1 || pv.Impact[0].Lose[0] != "web-1" {
		t.Fatalf("server preview: %d %+v", code, pv)
	}
	if !sees(sWeb) {
		t.Fatalf("a server preview must not move it")
	}

	// the delete: the preview says what moves; a name in the way refuses it; the children take the place of the folder
	var del struct {
		Subfolders int         `json:"subfolders"`
		Servers    int         `json:"servers"`
		Grants     []string    `json:"grants"`
		Impact     []impactRow `json:"impact"`
	}
	if code := admin.do("DELETE", "/api/groups/"+gid(prod.ID)+"?dry_run=1", nil, &del); code != 200 || del.Subfolders != 1 || del.Servers != 0 || len(del.Grants) != 1 || del.Grants[0] != "oper" || len(del.Impact) != 1 {
		t.Fatalf("delete preview: %d %+v", code, del)
	}
	if len(list()) != 11 {
		t.Fatalf("a delete preview must not delete: %d folders", len(list()))
	}
	// Web is in Prod; make Prod have a folder that would meet one of Web's children
	mk("L0", &prod.ID)
	if code := admin.do("POST", "/api/groups", map[string]any{"name": "Dup", "parent_id": web.ID}, nil); code != 200 {
		t.Fatalf("Dup under Web: %d", code)
	}
	mk("Dup", &prod.ID)
	if code := admin.do("DELETE", "/api/groups/"+gid(web.ID), nil, nil); code != http.StatusConflict {
		t.Fatalf("deleting Web would put its Dup next to the Dup of Prod: %d", code)
	}
	if got := kids(&prod.ID); got != "Web,L0,Dup," {
		t.Fatalf("a refused delete changes nothing: %s", got)
	}
	// Prod loses the clashing Dup, now Web can go: its Dup and its server go up, in the place of Web
	var dupProd int64
	for _, f := range list() {
		if f.Name == "Dup" && f.ParentID != nil && *f.ParentID == prod.ID {
			dupProd = f.ID
		}
	}
	if code := admin.do("PUT", "/api/groups/"+gid(dupProd), map[string]any{"name": "Dup-prod"}, nil); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	if code := admin.do("DELETE", "/api/groups/"+gid(web.ID), nil, &del); code != 200 || del.Subfolders != 1 || del.Servers != 1 {
		t.Fatalf("delete Web: %d %+v", code, del)
	}
	if got := kids(&prod.ID); got != "Dup,L0,Dup-prod," {
		t.Fatalf("the children take the place of the deleted folder: %s", got)
	}
	if !sees(sWeb) {
		t.Fatalf("the server of the deleted folder goes up to Prod and stays visible")
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action='group.delete'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit of the delete: %d %v", n, err)
	}
	// deleting Prod takes the access it gave: the operator has nothing left
	if code := admin.do("DELETE", "/api/groups/"+gid(prod.ID), nil, nil); code != 200 {
		t.Fatalf("delete Prod: %d", code)
	}
	if sees(sWeb) {
		t.Fatalf("the access given by a deleted folder is gone")
	}
	if got := kids(nil); got != "Dup,L0,Dup-prod,Test,Ops," {
		t.Fatalf("the children of Prod are on the top level now, where Prod was: %s", got)
	}

	// an operator reads the folders but cannot change them
	var l []folderRow
	if code := op.do("GET", "/api/groups", nil, &l); code != 200 || len(l) == 0 {
		t.Fatalf("an operator reads the folders: %d", code)
	}
	if code := op.do("PUT", "/api/groups/"+gid(test.ID)+"/move", map[string]any{"parent_id": nil}, nil); code != http.StatusForbidden {
		t.Fatalf("an operator must not move a folder: %d", code)
	}
}
