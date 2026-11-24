package config

import (
	"os"
	"path/filepath"
	"testing"
)

func valid() *Server {
	c := &Server{}
	c.Database.DSN = "postgres://x"
	c.Agents.Listen = "0.0.0.0:9443"
	c.applyDefaults()
	return c
}

func TestAgentPortIsMandatory(t *testing.T) {
	for _, mut := range []func(*Server){
		func(c *Server) { c.Agents.Listen = "" },
		func(c *Server) { c.Agents.Listen = "0.0.0.0" },
		func(c *Server) { c.Agents.Listen = "0.0.0.0:99999" },
	} {
		c := valid()
		mut(c)
		if c.Validate() == nil {
			t.Errorf("expected validation error for %+v", c)
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaults(t *testing.T) {
	c := valid()
	if c.Admin.CookieSecure != "auto" || c.Admin.LoginRate != 10 || c.DataDir != "/var/lib/linager" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestAppListenIsOptionalAndChecked(t *testing.T) {
	c := valid()
	if err := c.Validate(); err != nil {
		t.Fatalf("an installation without app.listen must stay valid: %v", err)
	}
	for _, bad := range []string{"9444", "0.0.0.0:0", "0.0.0.0:9443"} {
		c.App.Listen = bad
		if err := c.Validate(); err == nil {
			t.Errorf("app.listen %q must be refused", bad)
		}
	}
	c.App.Listen = "0.0.0.0:9444"
	if err := c.Validate(); err != nil {
		t.Fatalf("a good app.listen: %v", err)
	}
}

// A file written before the admin panel was removed still has its listen, base_path and trusted_proxies: it is read as it
// is, the keys are ignored.
func TestAnOldFileWithTheAdminPanelKeysStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	old := "data_dir: /var/lib/linager\ndatabase:\n  dsn: postgres://x\nadmin:\n  listen: \"0.0.0.0:8080\"\n  base_path: /linager\n  trusted_proxies: [\"10.0.0.0/8\"]\n  cookie_secure: always\napp:\n  listen: \"0.0.0.0:9444\"\nagents:\n  listen: \"0.0.0.0:9443\"\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadServer(path)
	if err != nil {
		t.Fatalf("an old file must load: %v", err)
	}
	if c.Admin.CookieSecure != "always" || c.App.Listen != "0.0.0.0:9444" || c.Agents.Listen != "0.0.0.0:9443" {
		t.Fatalf("what is still used is kept: %+v", c)
	}
}
