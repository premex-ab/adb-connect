package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/service"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Install/uninstall the background auto-connect watcher",
	}
	cmd.AddCommand(newServiceInstallCmd(), newServiceUninstallCmd())
	return cmd
}

func newServiceInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the watcher as a launchd (macOS) or systemd-user (Linux) service that auto-starts on login",
		RunE: func(cmd *cobra.Command, _ []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if err := service.Install(service.InstallOpts{BinaryPath: exe}); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Watcher installed. It will run at login and auto-connect phones as their wireless debugging comes online.")
			return nil
		},
	}
}

func newServiceUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the installed watcher service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			service.Uninstall()
			fmt.Fprintln(cmd.OutOrStdout(), "Watcher removed.")
			return nil
		},
	}
}
