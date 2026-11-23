package installer

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bolgov0zero/linager/internal/config"
)

func TestRenderedServerConfigIsValid(t *testing.T) {
	//nolint:gosec // fake DSN for rendering test
	p := &ServerParams{AgentPort: 9443, AdminMode: "proxy", AdminPort: 8080, DSN: "postgres://linager:x@127.0.0.1:5432/linager?sslmode=disable", DataDir: "/var/lib/linager"}
	out := RenderServerConfig(p, []string{"host", "10.0.0.5"})
	var c config.Server
	if err := yaml.Unmarshal([]byte(out), &c); err != nil {
		t.Fatal(err)
	}
	if c.Admin.Listen != "0.0.0.0:8080" || c.Agents.Listen != "0.0.0.0:9443" || len(c.Agents.Hosts) != 2 || c.Database.DSN != p.DSN {
		t.Fatalf("unexpected config: %+v", c)
	}
	p.AdminMode = "caddy"
	p.AdminPort = 41000
	if !strings.Contains(RenderServerConfig(p, nil), `"127.0.0.1:41000"`) {
		t.Fatal("caddy mode must listen on loopback only")
	}
}

func TestCaddyBlock(t *testing.T) {
	if got := RenderCaddyBlock("a.example.com", 443, 41000); !strings.Contains(got, "a.example.com {") || !strings.Contains(got, "reverse_proxy 127.0.0.1:41000") {
		t.Fatal(got)
	}
	if !strings.Contains(RenderCaddyBlock("a.example.com", 8443, 1), "a.example.com:8443 {") {
		t.Fatal("custom public port must be in the site address")
	}
}

func TestDefaultCaddyfileDetection(t *testing.T) {
	if !IsDefaultCaddyfile(":80 {\n\troot * /usr/share/caddy\n}") || !IsDefaultCaddyfile("") {
		t.Fatal("stock file must be recognized")
	}
	if IsDefaultCaddyfile("example.com {\n\treverse_proxy :3000\n}") {
		t.Fatal("user config must not be treated as default")
	}
}

func TestValidators(t *testing.T) {
	for in, ok := range map[string]bool{"443": true, "0": false, "65536": false, "abc": false, " 8080 ": true} {
		if _, got := ValidPort(in); got != ok {
			t.Errorf("ValidPort(%q)=%v", in, got)
		}
	}
	if !ValidDomain("linager.example.com") || ValidDomain("localhost") || ValidDomain("a b.com") {
		t.Error("ValidDomain")
	}
	if !ValidLogin("admin") || ValidLogin("a b") || ValidLogin("") {
		t.Error("ValidLogin")
	}
	if h, p, ok := SplitHostPort("10.0.0.5:9443"); !ok || h != "10.0.0.5" || p != 9443 {
		t.Error("SplitHostPort")
	}
	if _, _, ok := SplitHostPort("nohost"); ok {
		t.Error("SplitHostPort must reject missing port")
	}
}

func TestParseOSRelease(t *testing.T) {
	o := ParseOSRelease("PRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\nID=debian\nVERSION_ID=\"13\"\n")
	if o.ID != "debian" || o.VersionID != "13" || !o.Supported() {
		t.Fatalf("%+v", o)
	}
	if (OSInfo{ID: "debian", VersionID: "11"}).Supported() || (OSInfo{ID: "fedora", VersionID: "40"}).Supported() {
		t.Fatal("unsupported systems must be refused")
	}
	for _, v := range []string{"22.04", "24.04", "26.04"} {
		if !(OSInfo{ID: "ubuntu", VersionID: v}).Supported() {
			t.Errorf("ubuntu %s must be supported", v)
		}
	}
}

func TestRandomString(t *testing.T) {
	a, b := RandomString(32), RandomString(32)
	if len(a) != 32 || a == b {
		t.Fatal("random strings must be 32 chars and differ")
	}
}

func TestMergeCaddyfile(t *testing.T) {
	block := RenderCaddyBlock("a.example.com", 443, 41000)
	if got := mergeCaddyfile(":80 {\n\troot * /usr/share/caddy\n}", block); got != block {
		t.Fatal("stock Caddyfile must be replaced")
	}
	user := "site.example.com {\n\treverse_proxy :3000\n}\n"
	got := mergeCaddyfile(user, block)
	if !strings.HasPrefix(got, user) || !strings.Contains(got, "reverse_proxy 127.0.0.1:41000") {
		t.Fatal("user content must be preserved and the block appended")
	}
	again := mergeCaddyfile(got, RenderCaddyBlock("b.example.com", 443, 42000))
	if strings.Count(again, CaddyMarker) != 1 || strings.Contains(again, "41000") || !strings.Contains(again, "b.example.com") {
		t.Fatalf("re-running must replace the managed block:\n%s", again)
	}
}

func TestRenderedAppPort(t *testing.T) {
	p := &ServerParams{AgentPort: 9443, AppPort: 9444, AdminMode: "proxy", AdminPort: 8080, DSN: "postgres://x", DataDir: "/var/lib/linager"}
	var c config.Server
	if err := yaml.Unmarshal([]byte(RenderServerConfig(p, nil)), &c); err != nil || c.App.Listen != "0.0.0.0:9444" {
		t.Fatalf("app.listen: %v %+v", err, c.App)
	}
	p.AppPort = 0
	if strings.Contains(RenderServerConfig(p, nil), "app:") {
		t.Fatal("without an application port the file has no app block")
	}
}

// fakeConsole answers nothing: it stands for a run driven by flags.
type fakeConsole struct{}

func (fakeConsole) Interactive() bool                   { return false }
func (fakeConsole) Ask(Form) (map[string]string, error) { return nil, nil }
func (fakeConsole) Steps([]string)                      {}
func (fakeConsole) Event(Event)                         {}
func (fakeConsole) Wait(Waiting)                        {}
func (fakeConsole) Lang() Lang                          { return ParseLang("en") }

func TestServerParamsNeedAnApplicationPort(t *testing.T) {
	base := ServerOpts{AgentPort: 39443, AdminMode: "proxy", AdminPort: 39080, AdminUser: "admin"}
	_, err := collectServerParams(fakeConsole{}, base)
	if err == nil || !strings.Contains(err.Error(), "--app-port") {
		t.Fatalf("the application port is required, got %v", err)
	}
	same := base
	same.AppPort = base.AgentPort
	if _, err := collectServerParams(fakeConsole{}, same); err == nil {
		t.Fatal("the application port must differ from the agent port")
	}
	same.AppPort = base.AdminPort
	if _, err := collectServerParams(fakeConsole{}, same); err == nil {
		t.Fatal("the application port must differ from the admin port")
	}
	ok := base
	ok.AppPort = 39444
	ok.PasswordFile = ""
	p, err := collectServerParams(fakeConsole{}, ok)
	if err != nil || p.AppPort != 39444 {
		t.Fatalf("valid ports: %v %+v", err, p)
	}
}
