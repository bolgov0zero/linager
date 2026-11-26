//go:build !windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

const defaultServerConfig = "/etc/linager/server.yaml"

func newServerCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{Use: "server", Short: "Linager server"}
	cmd.PersistentFlags().StringVar(&cfgPath, "config", defaultServerConfig, "path to server.yaml")

	cmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run the server (the application port and the agent port)",
		RunE: func(*cobra.Command, []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return server.Run(ctx, cfg, slog.New(slog.NewTextHandler(os.Stderr, nil)))
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create missing keys and certificates in the data directory",
		RunE: func(*cobra.Command, []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			return server.InitKeys(cfg.DataDir, cfg.Agents.Hosts)
		},
	})

	cmd.AddCommand(newMigrateCmd(&cfgPath))

	var login, pwFile string
	create := &cobra.Command{
		Use:   "create-admin",
		Short: "Create the administrator account",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			pw, err := readSecretFile(pwFile)
			if err != nil {
				return err
			}
			db, err := store.Open(c.Context(), cfg.Database.DSN)
			if err != nil {
				return err
			}
			defer db.Close()
			return server.CreateAdmin(c.Context(), db, login, pw)
		},
	}
	create.Flags().StringVar(&login, "login", "", "administrator login")
	create.Flags().StringVar(&pwFile, "password-file", "", "file with the password ('-' for stdin)")
	_ = create.MarkFlagRequired("login")
	_ = create.MarkFlagRequired("password-file")
	cmd.AddCommand(create)

	appPort := &cobra.Command{
		Use:   "app-port [port]",
		Short: "Show the port and the certificate fingerprint of the desktop application, or set the port (restart the server after)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			db, err := store.Open(c.Context(), cfg.Database.DSN)
			if err != nil {
				return err
			}
			defer db.Close()
			out := c.OutOrStdout()
			if len(args) == 1 {
				port, perr := strconv.Atoi(args[0])
				if perr != nil {
					return fmt.Errorf("%q is not a port", args[0])
				}
				if err := server.SetAppPort(c.Context(), cfg, db, port); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(out, "The application port is %d. Restart the server to open it: systemctl restart linager\n", port)
			}
			info, err := server.AppPortInfo(c.Context(), cfg, db)
			if err != nil {
				return err
			}
			if info.Port == 0 {
				_, err = fmt.Fprintln(out, "The application port is not set: linager server app-port <port>")
				return err
			}
			_, err = fmt.Fprintf(out, "Application port: %d\nCertificate fingerprint: %s\n", info.Port, info.Fingerprint)
			if err == nil && info.FingerprintAlt != "" {
				// an iPhone or an iPad gets the other certificate: its fingerprint is what the phone shows
				_, err = fmt.Fprintf(out, "Fingerprint for iPhone and iPad (the ECDSA certificate): %s\n", info.FingerprintAlt)
			}
			if err == nil {
				_, err = fmt.Fprintf(out, "In the application enter <address of this server>:%d and compare the fingerprint.\n", info.Port)
			}
			return err
		},
	}
	cmd.AddCommand(appPort)

	devices := &cobra.Command{Use: "devices", Short: "Devices of the applications (the way back in when the only approved device is lost)"}
	withDB := func(run func(c *cobra.Command, db *store.Store, args []string) error) func(c *cobra.Command, args []string) error {
		return func(c *cobra.Command, args []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			db, err := store.Open(c.Context(), cfg.Database.DSN)
			if err != nil {
				return err
			}
			defer db.Close()
			return run(c, db, args)
		}
	}
	devices.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the devices and tell whether the confirmation of devices is on",
		Args:  cobra.NoArgs,
		RunE: withDB(func(c *cobra.Command, db *store.Store, _ []string) error {
			return server.DevicesList(c.Context(), db, c.OutOrStdout())
		}),
	})
	devices.AddCommand(&cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a device by its id (see: devices list)",
		Args:  cobra.ExactArgs(1),
		RunE: withDB(func(c *cobra.Command, db *store.Store, args []string) error {
			id, perr := strconv.ParseInt(args[0], 10, 64)
			if perr != nil {
				return fmt.Errorf("%q is not a device id", args[0])
			}
			if err := server.DeviceApproveCLI(c.Context(), db, id); err != nil {
				return err
			}
			_, err := fmt.Fprintln(c.OutOrStdout(), "The device is approved.")
			return err
		}),
	})
	devices.AddCommand(&cobra.Command{
		Use:   "off",
		Short: "Turn the confirmation of devices off (every device may sign in; turn it on again in the application)",
		Args:  cobra.NoArgs,
		RunE: withDB(func(c *cobra.Command, db *store.Store, _ []string) error {
			if err := server.DevicesGateOff(c.Context(), db); err != nil {
				return err
			}
			_, err := fmt.Fprintln(c.OutOrStdout(), "The confirmation of devices is off.")
			return err
		}),
	})
	cmd.AddCommand(devices)

	var rLogin, rPwFile string
	reset := &cobra.Command{
		Use:   "reset-password",
		Short: "Set a new password for an existing administrator (also lifts a lock-out and signs the user out)",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			pw, err := readSecretFile(rPwFile)
			if err != nil {
				return err
			}
			db, err := store.Open(c.Context(), cfg.Database.DSN)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := server.ResetPassword(c.Context(), db, rLogin, pw); err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), "The password was changed.")
			return err
		},
	}
	reset.Flags().StringVar(&rLogin, "login", "", "login of the user")
	reset.Flags().StringVar(&rPwFile, "password-file", "", "file with the new password ('-' for stdin)")
	_ = reset.MarkFlagRequired("login")
	_ = reset.MarkFlagRequired("password-file")
	cmd.AddCommand(reset)
	return cmd
}

func readSecretFile(path string) (string, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = readAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path) //nolint:gosec // operator-supplied path
	}
	if err != nil {
		return "", err
	}
	pw := strings.TrimRight(string(b), "\r\n")
	if pw == "" {
		return "", fmt.Errorf("password file is empty")
	}
	return pw, nil
}
