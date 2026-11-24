package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/bolgov0zero/linager/agent"
	"github.com/bolgov0zero/linager/internal/version"
)

// exitCodeConfig (EX_CONFIG) tells systemd not to restart the agent (RestartPreventExitStatus=78).
const exitCodeConfig = 78

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(io.LimitReader(r, 1<<16)) }

func newAgentCmd() *cobra.Command {
	var dataDir string
	cmd := &cobra.Command{Use: "agent", Short: "Linager agent"}
	cmd.PersistentFlags().StringVar(&dataDir, "data-dir", agent.DefaultDataDir, "agent data directory (keys, state)")

	var srv, fp, cfgPath, joinToken string
	enroll := &cobra.Command{
		Use:   "enroll",
		Short: "Pair this host with a Linager server and wait for approval",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			out := c.OutOrStdout()
			err := agent.Enroll(ctx, agent.EnrollOptions{
				Server: srv, Fingerprint: fp, DataDir: dataDir, AgentVersion: version.Version, JoinToken: joinToken,
				Confirm: func(f string) bool {
					_, _ = fmt.Fprintf(out, "Server fingerprint: %s\nTrust this server? [yes/no]: ", f)
					ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
					return strings.EqualFold(strings.TrimSpace(ans), "yes")
				},
				Show: func(key, code, f string) {
					_, _ = fmt.Fprintf(out, "\nKey:         %s\nCheck code:  %s\nFingerprint: %s\n\nEnter the key in the application and make sure the check code matches. Waiting...\n", key, code, f)
				},
			})
			if err == nil {
				_, _ = fmt.Fprintln(out, "Approved. The agent is enrolled.")
			}
			return err
		},
	}
	enroll.Flags().StringVar(&srv, "server", "", "server agent port as host:port")
	enroll.Flags().StringVar(&fp, "fingerprint", "", "expected server certificate fingerprint (SHA256:...)")
	enroll.Flags().StringVar(&joinToken, "join-token", "", "secret of an install link made in the application")
	_ = enroll.MarkFlagRequired("server")

	run := &cobra.Command{
		Use:   "run",
		Short: "Run the agent (connect to the server)",
		RunE:  func(c *cobra.Command, _ []string) error { return runAgent(c, dataDir, cfgPath) },
	}
	var purge bool
	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the agent identity and its service from this host",
		RunE: func(*cobra.Command, []string) error {
			return agent.Uninstall(purge, dataDir)
		},
	}
	uninstall.Flags().BoolVar(&purge, "purge", false, "also delete keys and configuration")
	run.Flags().StringVar(&cfgPath, "config", agent.DefaultConfigPath, "agent configuration (enabled modules)")
	cmd.AddCommand(enroll, run, uninstall)
	cmd.AddCommand(newAgentUpdateCmds(&dataDir)...)
	return cmd
}
