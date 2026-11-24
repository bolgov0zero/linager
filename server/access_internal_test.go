package server

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/bolgov0zero/linager/internal/config"
)

var specRe = regexp.MustCompile(`^(any|admin|srv|srv:admin|srv:(services|apt|disk|security|users|processes|files|network|firewall|docker|desktop|assistant)|mod:terminal)$`)

// Every authenticated route has an access rule, and the loose ones ("any", "srv") are only the ones that are
// meant to be: a new route cannot slip in open to an operator or an auditor by accident.
func TestRouteTableIsComplete(t *testing.T) {
	a := &Admin{Cfg: &config.Server{}}
	_ = a.Handler()
	if len(a.routes) < 100 {
		t.Fatalf("the route table looks empty: %d routes", len(a.routes))
	}
	anyOK := map[string]bool{
		"POST /api/logout": true, "GET /api/me": true, "POST /api/me/totp/setup": true, "POST /api/me/totp/enable": true,
		"POST /api/me/totp/disable": true, "POST /api/me/password": true, "POST /api/me/recovery-codes": true,
		"GET /api/servers": true, "GET /api/groups": true, "GET /api/dashboard": true,
		"PUT /api/dashboard/order": true, "GET /api/live": true,
	}
	srvOK := map[string]bool{
		"GET /api/servers/{id}": true, "GET /api/servers/{id}/tags": true, "GET /api/servers/{id}/metrics": true, "GET /api/servers/{id}/metrics.csv": true,
		"GET /api/servers/{id}/stream":                 true, // the module is checked by the kind of the stream
		"POST /api/servers/{id}/win/{module}/{action}": true, // the right is checked by the action (the table winActions)
	}
	seen := map[string]bool{}
	for _, r := range a.routes {
		if seen[r.Pattern] {
			t.Errorf("route %s is registered twice", r.Pattern)
		}
		seen[r.Pattern] = true
		if !specRe.MatchString(r.Spec) {
			t.Errorf("route %s has an unknown rule %q", r.Pattern, r.Spec)
		}
		path := r.Pattern[strings.IndexByte(r.Pattern, ' ')+1:]
		switch {
		case r.Spec == "any" && !anyOK[r.Pattern]:
			t.Errorf("route %s is open to every role: add it to the allowed list on purpose", r.Pattern)
		case r.Spec == "srv" && !srvOK[r.Pattern]:
			t.Errorf("route %s is open to auditors: add it to the allowed list on purpose", r.Pattern)
		case strings.HasPrefix(path, "/api/servers/{id}") && !strings.HasPrefix(r.Spec, "srv"):
			t.Errorf("route %s names a server but its rule %q does not check access to it", r.Pattern, r.Spec)
		}
	}
	for _, must := range []string{"DELETE /api/servers/{id}", "POST /api/servers/{id}/reboot", "POST /api/enrollments/{id}/approve", "GET /api/audit", "POST /api/users", "PUT /api/users/{id}/access", "POST /api/updates/agents"} {
		var spec string
		for _, r := range a.routes {
			if r.Pattern == must {
				spec = r.Spec
			}
		}
		if spec != "admin" && spec != "srv:admin" {
			t.Errorf("%s must be for administrators only, it has %q", must, spec)
		}
	}
}

func TestStreamModules(t *testing.T) {
	for kind, want := range map[string]string{"terminal": "terminal", "logs": "logs", "apt": "apt", "docker-logs": "docker", "docker-pull": "docker",
		"docker-update": "docker", "docker-project-logs": "docker", "f2b-install": "security", "bogus": ""} {
		if got := streamModule(kind); got != want {
			t.Errorf("%s: got %q want %q", kind, got, want)
		}
	}
	for _, m := range RootModules {
		if !validModule(m) {
			t.Errorf("%s is not an operator module", m)
		}
	}
}

// A terminal as another account: only a plain account name reaches the agent.
func TestTerminalUserParam(t *testing.T) {
	a := &Admin{}
	for query, ok := range map[string]bool{
		"kind=terminal":                                 true,
		"kind=terminal&user=deploy":                     true,
		"kind=terminal&user=ci.runner-2":                true,
		"kind=terminal&user=a%20b":                      false,
		"kind=terminal&user=%24%28id%29":                false,
		"kind=terminal&user=root%3Bls":                  false,
		"kind=terminal&user=" + strings.Repeat("a", 65): false,
	} {
		r := httptest.NewRequest("GET", "/api/servers/x/stream?"+query, nil)
		_, p, got := a.streamParams(r)
		if got != ok {
			t.Errorf("%s: ok=%v want %v", query, got, ok)
		}
		if ok && strings.Contains(query, "user=") && p["user"] == nil {
			t.Errorf("%s: the user must be passed to the agent", query)
		}
	}
}

// An apt run of chosen packages: only plain package names reach the agent.
func TestAptPackagesParam(t *testing.T) {
	a := &Admin{}
	for query, ok := range map[string]bool{
		"kind=apt&mode=packages&names=curl,libc6": true,
		"kind=apt&mode=packages&names=g%2B%2B":    true,
		"kind=apt&mode=packages":                  false,
		"kind=apt&mode=packages&names=curl,":      false,
		"kind=apt&mode=packages&names=-o%20x":     false,
		"kind=apt&mode=packages&names=a%3Bb":      false,
		"kind=apt&mode=upgrade":                   true,
	} {
		r := httptest.NewRequest("GET", "/api/servers/x/stream?"+query, nil)
		_, p, got := a.streamParams(r)
		if got != ok {
			t.Errorf("%s: ok=%v want %v", query, got, ok)
		}
		if ok && strings.Contains(query, "packages") && p["names"] == nil {
			t.Errorf("%s: the names must be passed to the agent", query)
		}
	}
}
