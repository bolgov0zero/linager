//go:build !windows

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/bolgov0zero/linager/installer"
)

func newInstallCmd() *cobra.Command {
	var o installer.Options
	var pwFile string
	cmd := &cobra.Command{
		Use:   "install [server|agent]",
		Short: "Interactive installer for the Linager server or agent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				o.Role = args[0]
			}
			o.Server.PasswordFile = pwFile
			o.Agent.Force = o.Server.Force
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return installer.Run(ctx, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Lang, "lang", "", "interface language: ru or en (default: from LANG)")
	f.BoolVar(&o.NoTUI, "no-tui", false, "plain line output, never ask questions")
	f.BoolVar(&o.Server.Force, "force", false, "reinstall over an existing installation")
	f.IntVar(&o.Server.AgentPort, "agent-port", 0, "server: port for clients (required, no default)")
	f.IntVar(&o.Server.AppPort, "app-port", 0, "server: port for the desktop application, the only way to the interface (required, no default)")
	f.StringVar(&o.Server.Domain, "domain", "", "server: a DNS name of this server for the certificate (optional)")
	f.StringVar(&o.Server.AdminUser, "admin-user", "", "server: administrator login (default admin)")
	f.StringVar(&pwFile, "admin-password-file", "", "server: file with the administrator password (empty = generate)")
	f.StringSliceVar(&o.Server.ExtraHosts, "agent-host", nil, "server: extra DNS name or IP for the client-port certificate")
	f.StringVar(&o.Agent.Server, "server", "", "agent: server address host:port of the client port")
	f.StringVar(&o.Agent.Fingerprint, "fingerprint", "", "agent: expected server fingerprint (SHA256:...)")
	f.StringVar(&o.Agent.JoinToken, "join-token", "", "agent: secret of an install link made in the application")
	f.StringVar(&o.Agent.DataDir, "data-dir", "", "agent: data directory (default /var/lib/linager-agent)")
	return cmd
}
