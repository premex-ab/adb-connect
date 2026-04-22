// Package main is the adb-connect CLI entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	flagVerbose bool
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "adb-connect",
		Short:         "Connect adb to an Android phone — same-LAN or over Tailscale",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose logging")
	root.AddCommand(
		newPairCmd(),
		newRemoteCmd(),
		newDaemonCmd(),
		newVersionCmd(),
	)
	return root
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		// Root has SilenceErrors=true so cobra doesn't print; print here so the user
		// sees why we exited instead of a mysteriously vanishing prompt.
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
