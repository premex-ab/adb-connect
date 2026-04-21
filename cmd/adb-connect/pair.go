package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/pair"
)

func newPairCmd() *cobra.Command {
	var timeout int
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Pair and connect an Android phone on the same Wi-Fi via a QR code",
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr, err := pair.Run(cmd.Context(), pair.Config{Open: pair.OpenBrowser})
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "pair: %s\n", err)
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "connected to %s\n", addr)
			return nil
		},
	}
	_ = timeout // reserved for future --timeout flag
	return cmd
}
