//go:build !windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/bolgov0zero/linager/agent"
	"github.com/bolgov0zero/linager/backup"
	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/release"
	"github.com/bolgov0zero/linager/internal/version"
	"github.com/bolgov0zero/linager/server/store"
	"github.com/bolgov0zero/linager/update"
	"github.com/bolgov0zero/linager/updater"
)

func newUpdateCmd() *cobra.Command {
	var cfgPath, channel, ver string
	var check, yes, applyRequest, allowDowngrade bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and install a new Linager server version (signed, with backup and automatic rollback)",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			d, cfg, err := updater.DefaultDeps(cfgPath)
			if err != nil {
				return fmt.Errorf("%w (use `linager agent update` on a client host)", err)
			}
			if channel == "" {
				channel = updater.ActiveChannel(ctx, cfg)
			}
			out := c.OutOrStdout()
			if applyRequest {
				return updater.ApplyRequest(ctx, d)
			}
			rels, err := d.Source.Releases(ctx)
			if err != nil {
				return err
			}
			target := ver
			if target == "" {
				latest, ok := update.Latest(rels, channel)
				if !ok {
					return errors.New("no release available on the " + channel + " channel")
				}
				target = latest.Tag
			}
			_, _ = fmt.Fprintf(out, "Current: %s\nAvailable (%s): %s\n", version.Version, channel, target)
			if rel, err := update.Find(rels, target); err == nil {
				for _, l := range update.NotesLines(rel.Notes) {
					_, _ = fmt.Fprintf(out, "  - %s\n", l)
				}
			}
			if check {
				if !release.Newer(target, version.Version) {
					_, _ = fmt.Fprintln(out, "Linager is up to date.")
				}
				return nil
			}
			if !release.Newer(target, version.Version) && !allowDowngrade {
				if ver == "" || release.Compare(release.ParseVersion(target), release.ParseVersion(version.Version)) == 0 {
					_, _ = fmt.Fprintln(out, "Linager is up to date.")
					return nil
				}
				return errors.New(target + " is older than the installed " + version.Version + "; pass --allow-downgrade if you really want it (the database is not migrated back)")
			}
			d.AllowDowngrade = allowDowngrade
			if !yes {
				_, _ = fmt.Fprintf(out, "Update %s -> %s? A database backup is taken first. [y/N]: ", version.Version, target)
				ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y") {
					return errors.New("cancelled")
				}
			}
			seen := map[string]string{}
			d.OnStatus = func(s updater.Status) {
				for _, st := range s.Steps {
					if seen[st.ID] != st.State && (st.State == "running" || st.State == "done" || st.State == "failed") {
						seen[st.ID] = st.State
						_, _ = fmt.Fprintf(out, "  %-8s %s\n", st.ID, st.State)
					}
				}
			}
			if err := updater.ServerUpdate(ctx, d, ver, channel); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Updated to", updater.ReadStatus(d.StatusPath).To)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&cfgPath, "config", defaultServerConfig, "path to server.yaml")
	f.StringVar(&channel, "channel", "", "stable or beta (default: the channel chosen in the application, else server.yaml)")
	f.StringVar(&ver, "version", "", "install this exact version")
	f.BoolVar(&check, "check", false, "only check, do not install")
	f.BoolVar(&yes, "yes", false, "do not ask for confirmation")
	f.BoolVar(&allowDowngrade, "allow-downgrade", false, "allow installing a version that is not newer than the current one")
	f.BoolVar(&applyRequest, "apply-request", false, "process the update request written by the application (used by systemd)")
	_ = f.MarkHidden("apply-request")
	return cmd
}

func newBackupCmd() *cobra.Command {
	var cfgPath, outDir string
	var keep int
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up the database (pg_dump) together with the configuration and keys",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			p, err := backup.Run(c.Context(), backup.Options{DSN: cfg.Database.DSN, OutDir: outDir, Files: []string{cfgPath}, DataDir: cfg.DataDir, Keep: keep})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), p)
			return nil
		},
	}
	cmd.Flags().StringVar(&cfgPath, "config", defaultServerConfig, "path to server.yaml")
	cmd.Flags().StringVar(&outDir, "output", backup.DefaultDir, "backup directory")
	cmd.Flags().IntVar(&keep, "keep", 0, "keep only the newest N backups (0 = keep all)")
	return cmd
}

func newRestoreCmd() *cobra.Command {
	var cfgPath string
	var withKeys, yes bool
	cmd := &cobra.Command{
		Use:   "restore <archive>",
		Short: "Restore the database (and optionally the keys) from a backup made by `linager backup`",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if !yes {
				_, _ = fmt.Fprintf(out, "Replace the current Linager database with %s? The server is stopped meanwhile. [y/N]: ", args[0])
				ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y") {
					return errors.New("cancelled")
				}
			}
			active := exec.CommandContext(c.Context(), "systemctl", "is-active", "--quiet", "linager-server").Run() == nil
			if active {
				if err := exec.CommandContext(c.Context(), "systemctl", "stop", "linager-server").Run(); err != nil {
					return fmt.Errorf("stop service: %w", err)
				}
				defer func() { _ = exec.Command("systemctl", "start", "linager-server").Run() }()
			}
			if err := backup.RestoreDB(c.Context(), args[0], cfg.Database.DSN); err != nil {
				return fmt.Errorf("the database was left unchanged: %w", err)
			}
			_, _ = fmt.Fprintln(out, "Database restored.")
			if withKeys {
				names, err := backup.RestoreKeys(args[0], cfg.DataDir)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(out, "Keys restored:", strings.Join(names, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cfgPath, "config", defaultServerConfig, "path to server.yaml")
	cmd.Flags().BoolVar(&withKeys, "with-keys", false, "also restore the CA, certificates and signing keys into the data directory")
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	return cmd
}

// newMigrateCmd applies pending database migrations (forward only); used by the updater.
func newMigrateCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending database migrations (forward only)",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.LoadServer(*cfgPath)
			if err != nil {
				return err
			}
			db, err := store.Open(c.Context(), cfg.Database.DSN)
			if err != nil {
				return err
			}
			db.Close()
			return nil
		},
	}
}

func newAgentUpdateCmds(dataDir *string) []*cobra.Command {
	var ver string
	upd := &cobra.Command{
		Use:   "update",
		Short: "Update this agent from the Linager server (signed release, automatic rollback)",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			out := c.OutOrStdout()
			if err := agent.UpdateFromServer(ctx, *dataDir, version.Version, ver, func(s string) { _, _ = fmt.Fprintln(out, s) }); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Installed. The agent restarts now and rolls back by itself if it cannot reconnect to the server in about a minute.")
			return nil
		},
	}
	upd.Flags().StringVar(&ver, "version", "", "install this exact version (default: the server's release)")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show the agent version and its connection state",
		RunE: func(c *cobra.Command, _ []string) error {
			_, _ = fmt.Fprint(c.OutOrStdout(), agent.StatusText(*dataDir, version.Version))
			return nil
		},
	}

	var exe string
	rollback := &cobra.Command{
		Use:    "rollback-update",
		Short:  "Restore the previous binary of an unconfirmed update (run by the systemd timer)",
		Hidden: true,
		RunE:   func(*cobra.Command, []string) error { return agent.RollbackUpdate(*dataDir, exe) },
	}
	rollback.Flags().StringVar(&exe, "binary", "/usr/local/bin/linager", "path of the installed binary")
	rollbackChange := &cobra.Command{
		Use:    "rollback-change",
		Short:  "Roll back an unconfirmed network or firewall change (run by the systemd timer)",
		Hidden: true,
		RunE: func(*cobra.Command, []string) error {
			return agent.RollbackPendingChange(*dataDir, slog.New(slog.NewTextHandler(os.Stderr, nil)))
		},
	}
	return append([]*cobra.Command{upd, status, rollback, rollbackChange}, assistHelperCmds()...)
}
