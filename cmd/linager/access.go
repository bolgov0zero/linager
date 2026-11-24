//go:build !windows

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/server"
	"github.com/bolgov0zero/linager/server/store"
)

// newAccessCmd is the way back when the access restriction of the admin panel has locked the admin out.
func newAccessCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "access",
		Short: "Access restrictions of the admin panel and of the application port",
	}
	reset := &cobra.Command{
		Use:   "reset",
		Short: "Turn the access restrictions of the admin panel and of the application port off (the lists are kept); a running server applies it within seconds",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.LoadServer(cfgPath)
			if err != nil {
				return err
			}
			db, err := store.Open(c.Context(), cfg.Database.DSN)
			if err != nil {
				return err
			}
			defer db.Close()
			changed, err := server.ResetAccess(c.Context(), db)
			if err != nil {
				return err
			}
			msg := "The access restriction was already off."
			if changed {
				msg = "The access restriction is off now. A running server applies it within a few seconds."
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), msg)
			return err
		},
	}
	reset.Flags().StringVar(&cfgPath, "config", defaultServerConfig, "path to server.yaml")
	cmd.AddCommand(reset)
	return cmd
}
