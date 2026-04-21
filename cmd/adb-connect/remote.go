package main

import "github.com/spf13/cobra"

func newRemoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Remote-ADB-over-Tailscale commands",
	}
	cmd.AddCommand(
		newRemoteSetupCmd(),
		newRemoteConnectCmd(),
		newRemoteStatusCmd(),
		newRemoteUninstallCmd(),
	)
	return cmd
}
