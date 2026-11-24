package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bolgov0zero/linager/server"
)

// TestAppAccessAPI: the list of the application port is read, checked and saved by an administrator only; it is validated, written to the audit, and the lists of host names are refused.
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
	}
	if code := admin.do("GET", "/api/app-access", nil, &st); code != 200 || st.Config.Mode != "off" {
		t.Fatalf("a new server: %d %+v", code, st)
	}
	in := map[string]any{"mode": "white", "white": []map[string]string{{"cidr": "10.0.0.9", "note": "office"}, {"cidr": "127.0.0.1"}}, "black": []map[string]string{}}
	if code := admin.do("PUT", "/api/app-access", in, &st); code != 200 || st.Config.Mode != "white" || len(st.Config.White) != 2 || st.Config.White[0].CIDR != "10.0.0.9/32" {
		t.Fatalf("save: %d %+v", code, st)
	}
	// a host name is accepted when DNS knows it, and refused (cannot be applied) when it does not
	server.SetHostResolver(&dnsTable{m: map[string][]string{"home.example.com": {"203.0.113.50"}}})
	t.Cleanup(func() { server.SetHostResolver(nil) })
	if code := admin.do("PUT", "/api/app-access", map[string]any{"mode": "black", "black": []map[string]string{{"cidr": "home.example.com"}}}, nil); code != 200 {
		t.Fatalf("a host name that DNS knows: %d", code)
	}
	var withHost struct {
		Hosts map[string]struct {
			IPs []string `json:"ips"`
		} `json:"hosts"`
	}
	if code := admin.do("GET", "/api/app-access", nil, &withHost); code != 200 || len(withHost.Hosts["home.example.com"].IPs) != 1 {
		t.Fatalf("the address of the name is shown: %d %+v", code, withHost)
	}
	if code := admin.do("PUT", "/api/app-access", map[string]any{"mode": "black", "black": []map[string]string{{"cidr": "nowhere.example.com"}}}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("a name without an address cannot be applied: %d", code)
	}
	// the reason of a failed lookup comes back with the refusal, so that the person sees what DNS of the server said
	var why struct {
		Status struct {
			Error string `json:"error"`
		} `json:"status"`
	}
	if code := admin.do("POST", "/api/app-access/resolve", map[string]string{"host": "nowhere.example.com"}, &why); code != http.StatusUnprocessableEntity || why.Status.Error == "" {
		t.Fatalf("resolve of an unknown name: %d %+v", code, why)
	}
	// back to the list of the rest of the test
	if code := admin.do("PUT", "/api/app-access", in, nil); code != 200 {
		t.Fatalf("restore: %d", code)
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
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'appaccess.update'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("an audit row for every saved change: %d %v", n, err)
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
	if err := json.Unmarshal(db.SettingRaw(context.Background(), "app_access"), &stored); err != nil || stored.Mode != "off" || len(stored.White) != 2 {
		t.Fatalf("the port is open again, the list is kept: %+v %v", stored, err)
	}
}
