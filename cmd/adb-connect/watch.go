package main

import (
	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/watch"
)

func newWatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Continuously browse mDNS and auto-connect to paired phones on the same Wi-Fi",
		Long: "Listens for _adb-tls-connect._tcp advertisements on the local network and runs " +
			"`adb connect` on each new device. Previously-paired phones attach to `adb devices` " +
			"automatically when their wireless debugging toggles on.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return watch.Run(cmd.Context(), watch.Config{})
		},
	}
}
