package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/adb"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/ipcserver"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/logger"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/wsserver"
	"github.com/premex-ab/adb-connect/internal/tailscale"
)

func newDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the remote-ADB daemon in the foreground (for launchd/systemd / debugging)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDaemon(cmd.Context())
		},
	}
}

func runDaemon(ctx context.Context) error {
	if err := os.MkdirAll(paths.ConfigDir(), 0o700); err != nil {
		return err
	}
	store, err := statestore.Open(paths.DBPath())
	if err != nil {
		return fmt.Errorf("open state store: %w", err)
	}
	defer store.Close()

	cfg, err := store.ServerConfig()
	if err != nil {
		return fmt.Errorf("read server config: %w", err)
	}
	if cfg == nil {
		return errors.New("no server config — run 'adb-connect remote setup' first")
	}

	bindHost := tailscale.IPv4(ctx)
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}

	log := logger.New()
	ws, err := wsserver.New(wsserver.Config{
		Store:    store,
		PSK:      string(cfg.PSK),
		BindHost: bindHost,
		BindPort: cfg.WSPort,
		Logger:   log,
	})
	if err != nil {
		return err
	}
	port, err := ws.Start()
	if err != nil {
		return err
	}
	log.Info("ws listening", "host", bindHost, "port", port)

	handlers := map[string]ipcserver.Handler{
		"status": func(_ map[string]any) (map[string]any, error) {
			phones, err := store.ListPhones()
			if err != nil {
				return nil, err
			}
			online := map[string]bool{}
			for _, n := range ws.OnlineNicknames() {
				online[n] = true
			}
			list := make([]map[string]any, 0, len(phones))
			for _, p := range phones {
				entry := map[string]any{
					"nickname":       p.Nickname,
					"tailscaleHost":  p.TailscaleHost,
					"paired":         boolToInt(p.Paired),
					"adbFingerprint": p.ADBFingerprint,
					"online":         online[p.Nickname],
				}
				if !p.LastWSSeen.IsZero() {
					entry["lastWsSeen"] = p.LastWSSeen.UnixMilli()
				}
				list = append(list, entry)
			}
			return map[string]any{
				"phones":        list,
				"bindHost":      bindHost,
				"wsPort":        cfg.WSPort,
				"tailscaleHost": cfg.TailscaleHost,
			}, nil
		},
		"connect": func(req map[string]any) (map[string]any, error) {
			nick, _ := req["nickname"].(string)
			phone, err := store.GetPhone(nick)
			if err != nil {
				return nil, err
			}
			if phone == nil {
				return nil, &wsserver.BusinessError{Code: "unknown_phone", Message: "unknown phone: " + nick}
			}
			ready, err := ws.RequestConnect(ctx, nick, !phone.Paired)
			if err != nil {
				return nil, err
			}
			if !phone.Paired {
				r, _ := adb.Pair(ctx, ready.IP, ready.Port, ready.PairCode)
				if !r.OK {
					return nil, &wsserver.BusinessError{Code: "pair_failed", Message: "adb pair failed: " + r.Stderr}
				}
				_ = store.MarkPaired(nick, "paired")
			}
			r, _ := adb.Connect(ctx, ready.IP, ready.Port)
			if !r.OK {
				return nil, &wsserver.BusinessError{Code: "connect_failed", Message: "adb connect failed: " + r.Stderr}
			}
			return map[string]any{
				"address": fmt.Sprintf("%s:%d", ready.IP, ready.Port),
				"message": "connected to " + fmt.Sprintf("%s:%d", ready.IP, ready.Port),
			}, nil
		},
	}
	ipc, err := ipcserver.New(handlers)
	if err != nil {
		return err
	}
	if err := ipc.Start(); err != nil {
		return err
	}
	log.Info("ipc listening", "socket", paths.IPCSocketPath())
	defer ipc.Stop()

	<-ctx.Done()
	log.Info("daemon shutting down")
	return ws.Stop()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
