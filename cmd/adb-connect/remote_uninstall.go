package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/service"
)

func newRemoteUninstallCmd() *cobra.Command {
	var wipeConfig bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Tear down the daemon service and (optionally) wipe config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			service.Uninstall()
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Daemon service removed.")
			if wipeConfig {
				if err := os.RemoveAll(paths.ConfigDir()); err != nil {
					return fmt.Errorf("wipe config: %w", err)
				}
				if err := os.RemoveAll(paths.LogDir()); err != nil {
					return fmt.Errorf("wipe logs: %w", err)
				}
				fmt.Fprintln(out, "Config and logs wiped.")
			} else {
				fmt.Fprintf(out, "Config kept at %s (use --wipe-config to remove)\n", paths.ConfigDir())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&wipeConfig, "wipe-config", false, "also delete the config directory (PSK, enrolled phones)")
	return cmd
}
