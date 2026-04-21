package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/ipcserver"
)

func newRemoteConnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect [nickname]",
		Short: "Connect to a remote phone by nickname (or auto-pick if only one is enrolled)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nickname, err := resolveNickname(args)
			if err != nil {
				return err
			}
			r, err := ipcserver.Request(map[string]any{"op": "connect", "nickname": nickname})
			if err != nil {
				return fmt.Errorf("ipc: %w", err)
			}
			if ok, _ := r["ok"].(bool); !ok {
				errMsg, _ := r["error"].(string)
				code, _ := r["code"].(string)
				if code != "" {
					return fmt.Errorf("(%s) %s", code, errMsg)
				}
				return errors.New(errMsg)
			}
			msg, _ := r["message"].(string)
			if msg == "" {
				if addr, _ := r["address"].(string); addr != "" {
					msg = "connected to " + addr
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), msg)
			return nil
		},
	}
}

func resolveNickname(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	r, err := ipcserver.Request(map[string]any{"op": "status"})
	if err != nil {
		return "", fmt.Errorf("ipc: %w", err)
	}
	phones, _ := r["phones"].([]any)
	if len(phones) == 0 {
		return "", errors.New("no phones enrolled. Run 'adb-connect remote setup' first.")
	}
	if len(phones) > 1 {
		var names []string
		for _, p := range phones {
			if m, ok := p.(map[string]any); ok {
				if n, ok := m["nickname"].(string); ok {
					names = append(names, n)
				}
			}
		}
		return "", fmt.Errorf("multiple phones enrolled (%v); pass a nickname", names)
	}
	m, _ := phones[0].(map[string]any)
	n, _ := m["nickname"].(string)
	return n, nil
}
