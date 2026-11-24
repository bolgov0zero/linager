package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

// peerServer serves the API of a second server over the same database (as after a restart; its reload loop is never started)
// and lets a test choose the address the connection comes from with the header X-Test-Peer: the application port judges the
// address of the connection, and on the loopback every connection is 127.0.0.1.
func peerServer(t *testing.T, db *store.Store) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	adm, err := server.NewAdmin(db, nil, nil, nil, nil, nil, &config.Server{}, log)
	if err != nil {
		t.Fatal(err)
	}
	h := adm.Handler()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.Header.Get("X-Test-Peer"); p != "" {
			r.RemoteAddr = net.JoinHostPort(p, "4000")
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// rawStatus calls the API without a session as the given address; /api/me answers 401 when the connection is let in and 403
// when the access restriction refuses it.
func rawStatus(t *testing.T, base, peer string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/me", nil)
	if peer != "" {
		req.Header.Set("X-Test-Peer", peer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

type accessProblems struct {
	Error    string `json:"error"`
	Problems []struct {
		Code   string `json:"code"`
		IP     string `json:"ip"`
		Reason string `json:"reason"`
	} `json:"problems"`
}

type appCfg struct {
	Mode  string              `json:"mode"`
	White []server.AccessRule `json:"white"`
	Black []server.AccessRule `json:"black"`
}

// The application port refuses the addresses the list does not let in, writes the refusal to the journal, refuses a list that
// would lock the caller out, and `linager access reset` opens it again.
func TestAppAccessEnforcement(t *testing.T) {
	_, db := startAccountsServer(t)
	srv := peerServer(t, db)
	admin := mustLogin(t, srv.URL, "admin", "a-long-test-password")

	var pr accessProblems
	if c := admin.do("PUT", "/api/app-access", appCfg{Mode: "white", White: []server.AccessRule{{CIDR: "203.0.113.0/24"}}}, &pr); c != http.StatusUnprocessableEntity || len(pr.Problems) != 1 || pr.Problems[0].Code != "lockout" {
		t.Fatalf("a list that locks the caller out: %d %+v", c, pr)
	}
	if c := admin.do("PUT", "/api/app-access", appCfg{Mode: "black", Black: []server.AccessRule{{CIDR: "127.0.0.1"}}}, &pr); c != http.StatusUnprocessableEntity {
		t.Fatalf("a black list with the caller: %d", c)
	}
	if c := admin.do("PUT", "/api/app-access", appCfg{Mode: "white"}, &pr); c != http.StatusUnprocessableEntity || pr.Problems[0].Code != "white_empty" {
		t.Fatalf("an empty white list: %d %+v", c, pr)
	}
	ok := appCfg{Mode: "white", White: []server.AccessRule{{CIDR: "127.0.0.1"}, {CIDR: "203.0.113.0/24", Note: "office"}}}
	if c := admin.do("PUT", "/api/app-access", ok, nil); c != 200 {
		t.Fatalf("apply: %d", c)
	}
	if got := rawStatus(t, srv.URL, "203.0.113.5"); got != http.StatusUnauthorized {
		t.Fatalf("a listed address passes: %d", got)
	}
	if got := rawStatus(t, srv.URL, "198.51.100.9"); got != http.StatusForbidden {
		t.Fatalf("an unlisted address is refused: %d", got)
	}
	var log struct {
		Rows []struct {
			ClientIP string `json:"client_ip"`
			Event    string `json:"event"`
		} `json:"rows"`
	}
	if c := admin.do("GET", "/api/app-access/log?filter=denied", nil, &log); c != 200 || len(log.Rows) != 1 || log.Rows[0].ClientIP != "198.51.100.9" || log.Rows[0].Event != "denied" {
		t.Fatalf("the refusal is in the journal: %d %+v", c, log)
	}

	// a second server over the same database (a restart) judges by the stored list from its first request
	again := peerServer(t, db)
	if rawStatus(t, again.URL, "198.51.100.9") != http.StatusForbidden || rawStatus(t, again.URL, "203.0.113.5") != http.StatusUnauthorized {
		t.Fatal("a restarted server must apply the stored list")
	}

	// the way back after a lock-out: the restriction is off, the list stays
	changed, err := server.ResetAccess(context.Background(), db)
	if err != nil || !changed {
		t.Fatalf("reset: %v %v", changed, err)
	}
	after := peerServer(t, db)
	if rawStatus(t, after.URL, "198.51.100.9") != http.StatusUnauthorized {
		t.Fatal("after the reset the port is open")
	}
}

// The lists of the admin panel restriction (removed with the panel) are carried to the application port once, so that a port
// that was closed stays closed; a port that has lists of its own is not touched.
func TestAdminPanelListsAreCarriedToTheApplicationPort(t *testing.T) {
	_, db := startAccountsServer(t)
	ctx := context.Background()
	legacy := `{"mode":"white","only_proxy":true,"white":[{"cidr":"203.0.113.0/24","note":"office"},{"cidr":"home.example.org","note":"home"}],"black":[],"proxies":[{"cidr":"10.0.0.1","note":""}],"retention_days":30}`
	if err := db.SetSettingRaw(ctx, "access", []byte(legacy)); err != nil {
		t.Fatal(err)
	}
	server.SetHostResolver(&dnsTable{m: map[string][]string{"home.example.org": {"198.51.100.20"}}})
	t.Cleanup(func() { server.SetHostResolver(nil) })
	srv := peerServer(t, db)
	if got := rawStatus(t, srv.URL, "203.0.113.5"); got != http.StatusUnauthorized {
		t.Fatalf("a listed address passes: %d", got)
	}
	if got := rawStatus(t, srv.URL, "192.0.2.77"); got != http.StatusForbidden {
		t.Fatalf("a port that was closed stays closed: %d", got)
	}
	var stored appCfg
	if err := json.Unmarshal(db.SettingRaw(ctx, "app_access"), &stored); err != nil || stored.Mode != "white" || len(stored.White) != 2 || stored.White[1].CIDR != "home.example.org" {
		t.Fatalf("carried to the application port with the names, without the proxies: %+v %v", stored, err)
	}

	// lists of its own win: a later change of the old setting does nothing
	if err := db.SetSettingRaw(ctx, "app_access", []byte(`{"mode":"off","white":[],"black":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSettingRaw(ctx, "access", []byte(`{"mode":"white","white":[{"cidr":"10.9.9.9"}]}`)); err != nil {
		t.Fatal(err)
	}
	if got := rawStatus(t, peerServer(t, db).URL, "192.0.2.77"); got != http.StatusUnauthorized {
		t.Fatalf("the saved lists of the port are not replaced: %d", got)
	}
	// an old restriction that was off carries nothing
	if err := db.SetSettingRaw(ctx, "app_access", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}

// dnsTable is a resolver the test controls.
type dnsTable struct {
	mu   sync.Mutex
	m    map[string][]string
	fail bool
}

func (d *dnsTable) set(host string, ips ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.m[host] = ips
}

func (d *dnsTable) setFail(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fail = v
}

func (d *dnsTable) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ips, ok := d.m[host]
	if d.fail || !ok {
		return nil, errors.New("no such host")
	}
	var out []netip.Addr
	for _, s := range ips {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

// A host name (DDNS) in the white list stands for the addresses it points to now: a new address takes effect when the
// name is refreshed, a DNS outage keeps the last known address, and the last known address survives a restart.
func TestAppAccessHostNames(t *testing.T) {
	dns := &dnsTable{m: map[string][]string{"home.example.org": {"203.0.113.50"}}}
	server.SetHostResolver(dns)
	t.Cleanup(func() { server.SetHostResolver(nil) })
	_, db := startAccountsServer(t)
	srv := peerServer(t, db)
	admin := mustLogin(t, srv.URL, "admin", "a-long-test-password")

	type resolved struct {
		Host   string            `json:"host"`
		Status server.HostStatus `json:"status"`
		Error  string            `json:"error"`
	}
	var rs resolved
	if c := admin.do("POST", "/api/app-access/resolve", map[string]string{"host": "nx.example.org"}, &rs); c != http.StatusUnprocessableEntity || rs.Error != "unresolved" || rs.Status.Error == "" {
		t.Fatalf("a name that does not exist (the answer of DNS is shown): %d %+v", c, rs)
	}
	if c := admin.do("POST", "/api/app-access/resolve", map[string]string{"host": "not a name"}, nil); c != http.StatusBadRequest {
		t.Fatalf("a bad name: %d", c)
	}
	if c := admin.do("POST", "/api/app-access/resolve", map[string]string{"host": "Home.Example.org"}, &rs); c != 200 || rs.Host != "home.example.org" || len(rs.Status.IPs) != 1 || rs.Status.IPs[0] != "203.0.113.50" {
		t.Fatalf("resolve: %d %+v", c, rs)
	}

	// a name that does not resolve must not be applied; the check says which one
	var pr accessProblems
	cfg := appCfg{Mode: "white", White: []server.AccessRule{{CIDR: "127.0.0.1"}, {CIDR: "nx.example.org"}}}
	if c := admin.do("PUT", "/api/app-access", cfg, &pr); c != http.StatusUnprocessableEntity || len(pr.Problems) != 1 || pr.Problems[0].Code != "host_unresolved" {
		t.Fatalf("unresolved name: %d %+v", c, pr)
	}

	var st struct {
		Config appCfg                       `json:"config"`
		Hosts  map[string]server.HostStatus `json:"hosts"`
	}
	cfg.White[1] = server.AccessRule{CIDR: "Home.Example.org", Note: "my home"}
	if c := admin.do("PUT", "/api/app-access", cfg, &st); c != 200 || st.Config.White[1].CIDR != "home.example.org" || len(st.Hosts["home.example.org"].IPs) != 1 {
		t.Fatalf("apply: %d %+v", c, st)
	}
	if rawStatus(t, srv.URL, "203.0.113.50") != http.StatusUnauthorized {
		t.Fatal("the address of the name must pass")
	}
	if rawStatus(t, srv.URL, "203.0.113.51") != http.StatusForbidden {
		t.Fatal("another address must be refused")
	}
	if raw := db.SettingRaw(context.Background(), "access_hosts"); !strings.Contains(string(raw), "203.0.113.50") {
		t.Fatalf("the answer must be stored: %s", raw)
	}

	// DNS is down: the last known address stays in force and the status says so
	dns.setFail(true)
	if c := admin.do("POST", "/api/app-access/resolve", map[string]string{"host": "home.example.org"}, &rs); c != 200 || !rs.Status.Failing || len(rs.Status.IPs) != 1 {
		t.Fatalf("outage: %d %+v", c, rs)
	}
	if rawStatus(t, srv.URL, "203.0.113.50") != http.StatusUnauthorized {
		t.Fatal("the last known address must stay in force")
	}

	// a restarted server (DNS still down) knows the last address from the database
	again := peerServer(t, db)
	if rawStatus(t, again.URL, "203.0.113.50") != http.StatusUnauthorized || rawStatus(t, again.URL, "203.0.113.51") != http.StatusForbidden {
		t.Fatal("a restarted server must judge by the stored address")
	}

	// the provider gave the home a new address; refreshing the name puts it in force at once
	dns.setFail(false)
	dns.set("home.example.org", "203.0.113.51")
	if c := admin.do("POST", "/api/app-access/resolve", map[string]string{"host": "home.example.org"}, &rs); c != 200 || rs.Status.IPs[0] != "203.0.113.51" || rs.Status.Failing {
		t.Fatalf("refresh: %d %+v", c, rs)
	}
	if rawStatus(t, srv.URL, "203.0.113.51") != http.StatusUnauthorized || rawStatus(t, srv.URL, "203.0.113.50") != http.StatusForbidden {
		t.Fatal("the new address must replace the old one")
	}
}
