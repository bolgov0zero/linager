package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bolgov0zero/linager/server"
)

// TestAppAccessAPI: the list of the application port is read, checked and saved by an administrator only; it is stored apart
// from the admin panel, validated, written to the audit, and the lists of host names are refused.
func TestAppAccessAPI(t *testing.T) {
	base, db := startAccountsServer(t)
	admin := mustLogin(t, base, "admin", "a-long-test-password")

	var st struct {
		Config struct {
			Mode  string `json:"mode"`
			White []struct {
				CIDR string `json:"cidr"`
				Note string `json:"note"`
			} `json:"white"`
		} `json:"config"`
		Configured bool `json:"configured"`
	}
	if code := admin.do("GET", "/api/app-access", nil, &st); code != 200 || st.Configured || st.Config.Mode != "off" {
		t.Fatalf("a new server: %d %+v", code, st)
	}
	in := map[string]any{"mode": "white", "white": []map[string]string{{"cidr": "10.0.0.9", "note": "office"}}, "black": []map[string]string{}}
	if code := admin.do("PUT", "/api/app-access", in, &st); code != 200 || !st.Configured || st.Config.Mode != "white" || len(st.Config.White) != 1 || st.Config.White[0].CIDR != "10.0.0.9/32" {
		t.Fatalf("save: %d %+v", code, st)
	}
	// the admin panel keeps its own restriction
	var adm struct {
		Config struct {
			Mode string `json:"mode"`
		} `json:"config"`
	}
	if code := admin.do("GET", "/api/access", nil, &adm); code != 200 || adm.Config.Mode != "off" {
		t.Fatalf("the admin panel restriction is not touched: %d %+v", code, adm)
	}
	if code := admin.do("PUT", "/api/app-access", map[string]any{"mode": "black", "black": []map[string]string{{"cidr": "home.example.com"}}}, nil); code != http.StatusBadRequest {
		t.Fatalf("a host name is refused: %d", code)
	}
	if code := admin.do("PUT", "/api/app-access", map[string]any{"mode": "sometimes"}, nil); code != http.StatusBadRequest {
		t.Fatalf("an unknown mode: %d", code)
	}
	var chk struct {
		Test *struct {
			Allowed bool `json:"allowed"`
		} `json:"test"`
	}
	if code := admin.do("POST", "/api/app-access/check", map[string]any{"config": in, "test_ip": "10.0.0.9"}, &chk); code != 200 || chk.Test == nil || !chk.Test.Allowed {
		t.Fatalf("check, listed address: %d %+v", code, chk)
	}
	chk.Test = nil
	if code := admin.do("POST", "/api/app-access/check", map[string]any{"config": in, "test_ip": "203.0.113.5"}, &chk); code != 200 || chk.Test == nil || chk.Test.Allowed {
		t.Fatalf("check, other address: %d %+v", code, chk)
	}
	var n int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'appaccess.update'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("one audit row: %d %v", n, err)
	}
	// `linager access reset` turns the restriction of the port off as well (the way back after a lock-out)
	changed, err := server.ResetAccess(context.Background(), db)
	if err != nil || !changed {
		t.Fatalf("reset: %v %v", changed, err)
	}
	var stored struct {
		Mode  string `json:"mode"`
		White []struct {
			CIDR string `json:"cidr"`
		} `json:"white"`
	}
	if err := json.Unmarshal(db.SettingRaw(context.Background(), "app_access"), &stored); err != nil || stored.Mode != "off" || len(stored.White) != 1 {
		t.Fatalf("the port is open again, the list is kept: %+v %v", stored, err)
	}
}
