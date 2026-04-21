package main

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/ipcserver"
)

func newRemoteStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon state + enrolled phones",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := ipcserver.Request(map[string]any{"op": "status"})
			if err != nil {
				return fmt.Errorf("ipc: %w", err)
			}
			if ok, _ := r["ok"].(bool); !ok {
				errMsg, _ := r["error"].(string)
				return fmt.Errorf("%s", errMsg)
			}
			out := cmd.OutOrStdout()
			bindHost, _ := r["bindHost"].(string)
			wsPort, _ := r["wsPort"].(float64)
			tsHost, _ := r["tailscaleHost"].(string)
			fmt.Fprintf(out, "Daemon:       %s:%d\nTailnet host: %s\n\n", bindHost, int(wsPort), tsHost)
			phones, _ := r["phones"].([]any)
			if len(phones) == 0 {
				fmt.Fprintln(out, "(no phones enrolled)")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "STATUS\tNICKNAME\tPAIRED\tLAST SEEN")
			for _, p := range phones {
				m, _ := p.(map[string]any)
				nick, _ := m["nickname"].(string)
				paired, _ := m["paired"].(float64)
				online, _ := m["online"].(bool)
				lastMS, _ := m["lastWsSeen"].(float64)
				status := "offline"
				if online {
					status = "online"
				}
				pairedStr := "no"
				if paired == 1 {
					pairedStr = "yes"
				}
				lastSeen := "never"
				if lastMS > 0 {
					lastSeen = time.UnixMilli(int64(lastMS)).Format(time.RFC3339)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", status, nick, pairedStr, lastSeen)
			}
			return tw.Flush()
		},
	}
}
