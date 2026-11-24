// Package config loads and validates the server configuration file.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Server is the content of /etc/linager/server.yaml.
type Server struct {
	DataDir  string `yaml:"data_dir"`
	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`
	// Admin keeps the settings of sign-in. The admin panel (and its listen, base_path and trusted_proxies keys) is gone: an old
	// file that still has them is read as it is, they are ignored.
	Admin struct {
		CookieSecure string `yaml:"cookie_secure"` // auto|always|never
		RequireTOTP  bool   `yaml:"require_totp"`  // force the administrator to enable 2FA before using the interface
		LoginRate    int    `yaml:"login_rate"`    // sign-in attempts per minute per address (default 10)
	} `yaml:"admin"`
	Update struct {
		Source  string `yaml:"source"`  // "" = GitHub releases of bolgov0zero/linager, "owner/repo", or a mirror URL
		Channel string `yaml:"channel"` // stable (default) or beta
	} `yaml:"update"`
	// SSHInstall limits where the panel may open SSH connections when it installs an agent. Empty means any address
	// except loopback, link-local (cloud metadata) and multicast, which are always refused.
	SSHInstall struct {
		AllowedNetworks []string `yaml:"allowed_networks"`
	} `yaml:"ssh_install"`
	// App is the listener of the desktop application: TLS with the certificate of the agent port. It is the only way to the
	// interface (the port can be changed in the application or with `linager server app-port`).
	App struct {
		Listen string `yaml:"listen"` // host:port
	} `yaml:"app"`
	Agents struct {
		Listen string   `yaml:"listen"` // host:port, port is mandatory
		Hosts  []string `yaml:"hosts"`  // names/IPs placed into the agent TLS certificate
		// CertLifetime is how long an agent certificate is valid (Go duration, default 8760h = one year).
		// Agents renew it by themselves when a third of the lifetime is left.
		CertLifetime string `yaml:"cert_lifetime"`
	} `yaml:"agents"`
}

// DefaultAgentCertLifetime is used when agents.cert_lifetime is not set.
const DefaultAgentCertLifetime = 365 * 24 * time.Hour

// AgentCertLifetime returns the configured validity of agent certificates.
func (c *Server) AgentCertLifetime() time.Duration {
	if d, err := time.ParseDuration(c.Agents.CertLifetime); err == nil && d > 0 {
		return d
	}
	return DefaultAgentCertLifetime
}

// LoadServer reads, defaults and validates a server config.
func LoadServer(path string) (*Server, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is an operator-supplied config file
	if err != nil {
		return nil, err
	}
	var c Server
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.applyDefaults()
	return &c, c.Validate()
}

func (c *Server) applyDefaults() {
	if c.DataDir == "" {
		c.DataDir = "/var/lib/linager"
	}
	if c.Admin.LoginRate <= 0 {
		c.Admin.LoginRate = 10
	}
	if c.Admin.CookieSecure == "" {
		c.Admin.CookieSecure = "auto"
	}
	if c.Update.Channel != "beta" {
		c.Update.Channel = "stable"
	}
}

// Validate checks required settings. There are deliberately no default ports.
func (c *Server) Validate() error {
	if c.Database.DSN == "" {
		return errors.New("database.dsn is required")
	}
	if err := validListen(c.Agents.Listen); err != nil {
		return fmt.Errorf("agents.listen: %w", err)
	}
	if c.App.Listen != "" {
		if err := validListen(c.App.Listen); err != nil {
			return fmt.Errorf("app.listen: %w", err)
		}
		if c.App.Listen == c.Agents.Listen {
			return errors.New("app.listen and agents.listen must differ")
		}
	}
	if v := c.Agents.CertLifetime; v != "" {
		if d, err := time.ParseDuration(v); err != nil || d < time.Minute {
			return errors.New("agents.cert_lifetime must be a duration of at least 1m, for example 8760h")
		}
	}
	switch c.Admin.CookieSecure {
	case "auto", "always", "never":
	default:
		return errors.New("admin.cookie_secure must be auto, always or never")
	}
	for _, n := range c.SSHInstall.AllowedNetworks {
		if _, err := netip.ParsePrefix(n); err != nil {
			return fmt.Errorf("ssh_install.allowed_networks: %q is not a network like 10.0.0.0/8", n)
		}
	}
	return nil
}

func validListen(addr string) error {
	if addr == "" {
		return errors.New("is required (no default port)")
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return errors.New("invalid port")
	}
	return nil
}
