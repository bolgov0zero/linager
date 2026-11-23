package config

import "testing"

func valid() *Server {
	c := &Server{}
	c.Database.DSN = "postgres://x"
	c.Admin.Listen, c.Agents.Listen = "0.0.0.0:8080", "0.0.0.0:9443"
	c.applyDefaults()
	return c
}

func TestPortsAreMandatory(t *testing.T) {
	for _, mut := range []func(*Server){
		func(c *Server) { c.Admin.Listen = "" },
		func(c *Server) { c.Agents.Listen = "" },
		func(c *Server) { c.Admin.Listen = "0.0.0.0" },
		func(c *Server) { c.Agents.Listen = "0.0.0.0:99999" },
		func(c *Server) { c.Agents.Listen = c.Admin.Listen },
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
	if len(c.Admin.TrustedProxies) != 2 || c.Admin.CookieSecure != "auto" || c.DataDir != "/var/lib/linager" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestAppListenIsOptionalAndChecked(t *testing.T) {
	c := &Server{}
	c.Database.DSN = "postgres://x"
	c.Admin.Listen, c.Agents.Listen = "0.0.0.0:8080", "0.0.0.0:9443"
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		t.Fatalf("an installation without app.listen must stay valid: %v", err)
	}
	for _, bad := range []string{"9444", "0.0.0.0:0", "0.0.0.0:8080", "0.0.0.0:9443"} {
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
