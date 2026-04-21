package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the adb-connect version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.Full())
			return nil
		},
	}
}
