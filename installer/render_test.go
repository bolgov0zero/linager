package installer

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/bolgov0zero/linager/internal/config"
)

func TestRenderedServerConfigIsValid(t *testing.T) {
	//nolint:gosec // fake DSN for rendering test
	p := &ServerParams{AgentPort: 9443, AppPort: 9444, DSN: "postgres://linager:x@127.0.0.1:5432/linager?sslmode=disable", DataDir: "/var/lib/linager"}
	out := RenderServerConfig(p, []string{"host", "10.0.0.5"})
	var c config.Server
	if err := yaml.Unmarshal([]byte(out), &c); err != nil {
		t.Fatal(err)
	}
	if c.Agents.Listen != "0.0.0.0:9443" || c.App.Listen != "0.0.0.0:9444" || len(c.Agents.Hosts) != 2 || c.Database.DSN != p.DSN {
		t.Fatalf("unexpected config: %+v", c)
	}
	if strings.Contains(out, "listen: \"0.0.0.0:8080\"") || strings.Contains(out, "trusted_proxies") || strings.Contains(out, "base_path") {
		t.Fatalf("the admin panel is gone, its keys are not written:\n%s", out)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("the rendered file must be valid: %v", err)
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

func TestRenderedAppPort(t *testing.T) {
	p := &ServerParams{AgentPort: 9443, AppPort: 9444, DSN: "postgres://x", DataDir: "/var/lib/linager"}
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
	base := ServerOpts{AgentPort: 39443, AdminUser: "admin"}
	_, err := collectServerParams(fakeConsole{}, base)
	if err == nil || !strings.Contains(err.Error(), "--app-port") {
		t.Fatalf("the application port is required, got %v", err)
	}
	same := base
	same.AppPort = base.AgentPort
	if _, err := collectServerParams(fakeConsole{}, same); err == nil {
		t.Fatal("the application port must differ from the agent port")
	}
	bad := base
	bad.AppPort, bad.Domain = 39444, "not a domain"
	if _, err := collectServerParams(fakeConsole{}, bad); err == nil {
		t.Fatal("a DNS name for the certificate must be a domain")
	}
	ok := base
	ok.AppPort = 39444
	ok.PasswordFile = ""
	p, err := collectServerParams(fakeConsole{}, ok)
	if err != nil || p.AppPort != 39444 {
		t.Fatalf("valid ports: %v %+v", err, p)
	}
}
