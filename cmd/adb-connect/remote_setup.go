package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/pair"
	"github.com/premex-ab/adb-connect/internal/remote/bootstrap"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/logger"
	"github.com/premex-ab/adb-connect/internal/version"
)

func newRemoteSetupCmd() *cobra.Command {
	var fromSource bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "One-time bootstrap: install Tailscale, daemon, Premex ADB-gate app, enrol a phone",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return bootstrap.RemoteSetup(cmd.Context(), bootstrap.RemoteSetupOpts{
				Version:     version.Version,
				FromSource:  fromSource,
				Input:       os.Stdin,
				Output:      cmd.OutOrStdout(),
				OpenBrowser: pair.OpenBrowser,
				Logger:      logger.New(),
			})
		},
	}
	cmd.Flags().BoolVar(&fromSource, "from-source", false, "build the Android app from source (requires ANDROID_HOME) instead of downloading the signed APK")
	return cmd
}
