// Package watch continuously browses _adb-tls-connect._tcp on the local mDNS
// domain and calls `adb connect` for each newly-seen Android phone that is
// already paired. Phones that were never paired will fail the connect — that
// is expected and logged at info level.
package watch

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/grandcat/zeroconf"

	"github.com/premex-ab/adb-connect/internal/adb"
)

// adbDevice mirrors adb.Device but is declared locally so the test seam
// does not depend on the concrete adb package.
type adbDevice struct {
	Serial string
	State  string
}

// Config controls the watcher. All fields are optional; nil seam functions are
// replaced with real implementations before Run enters its loop.
type Config struct {
	Logger *slog.Logger

	// Test-only seams — leave nil in production.
	browse          func(ctx context.Context, entries chan<- *zeroconf.ServiceEntry) error
	adbConnectFn    func(ctx context.Context, host string, port int) error
	adbDisconnectFn func(ctx context.Context, host string, port int) error
	adbDevicesFn    func(ctx context.Context) ([]adbDevice, error)
}

// Run starts the mDNS watch loop. It blocks until ctx is cancelled, then
// returns nil. On ctx cancel it returns nil (graceful shutdown).
func Run(ctx context.Context, cfg Config) error {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.browse == nil {
		cfg.browse = defaultBrowse
	}
	if cfg.adbConnectFn == nil {
		cfg.adbConnectFn = defaultAdbConnect
	}
	if cfg.adbDisconnectFn == nil {
		cfg.adbDisconnectFn = defaultAdbDisconnect
	}
	if cfg.adbDevicesFn == nil {
		cfg.adbDevicesFn = defaultAdbDevices
	}

	entries := make(chan *zeroconf.ServiceEntry, 32)
	go func() {
		if err := cfg.browse(ctx, entries); err != nil && ctx.Err() == nil {
			cfg.Logger.Warn("mDNS browse error", "err", err)
		}
	}()

	// connected tracks instance names we have successfully adb-connected.
	// Value is the "host:port" string used so reconcile can compare with
	// `adb devices` serials.
	connected := map[string]string{} // instance → "host:port"

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	cfg.Logger.Info("adb-connect watch started — listening for _adb-tls-connect._tcp")

	for {
		select {
		case <-ctx.Done():
			cfg.Logger.Info("adb-connect watch stopped")
			return nil

		case e, ok := <-entries:
			if !ok {
				cfg.Logger.Info("mDNS entries channel closed")
				return nil
			}
			handleEntry(ctx, cfg, connected, e)

		case <-ticker.C:
			reconcile(ctx, cfg, connected)
		}
	}
}

// handleEntry tries to adb-connect a newly seen (or re-announced) mDNS entry.
func handleEntry(ctx context.Context, cfg Config, connected map[string]string, e *zeroconf.ServiceEntry) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if e == nil {
		return
	}

	host := ""
	if len(e.AddrIPv4) > 0 {
		host = e.AddrIPv4[0].String()
	} else if len(e.AddrIPv6) > 0 {
		host = "[" + e.AddrIPv6[0].String() + "]"
	} else {
		cfg.Logger.Debug("mDNS entry has no addresses, skipping", "instance", e.ServiceInstanceName())
		return
	}
	port := e.Port
	hostPort := fmt.Sprintf("%s:%d", host, port)

	// Dedup: if already connected at the same host:port, skip. If the instance's
	// port changed (Android rotates the wireless-debugging port on every toggle
	// cycle), disconnect the stale endpoint so `adb devices` doesn't accumulate
	// offline entries, then fall through to the fresh connect.
	if prev, ok := connected[e.ServiceInstanceName()]; ok {
		if prev == hostPort {
			cfg.Logger.Debug("already connected, skipping", "instance", e.ServiceInstanceName(), "addr", hostPort)
			return
		}
		cfg.Logger.Info("mDNS port rotated, disconnecting stale endpoint",
			"instance", e.ServiceInstanceName(), "old", prev, "new", hostPort)
		if prevHost, prevPort, ok := splitHostPort(prev); ok {
			if err := cfg.adbDisconnectFn(ctx, prevHost, prevPort); err != nil {
				cfg.Logger.Debug("adb disconnect of stale endpoint failed (ignored)", "addr", prev, "err", err)
			}
		}
		delete(connected, e.ServiceInstanceName())
	}

	cfg.Logger.Info("new mDNS device, attempting adb connect", "instance", e.ServiceInstanceName(), "addr", hostPort)

	if err := cfg.adbConnectFn(ctx, host, port); err != nil {
		cfg.Logger.Warn("adb connect failed (phone may not be paired)", "instance", e.ServiceInstanceName(), "addr", hostPort, "err", err)
		return
	}

	connected[e.ServiceInstanceName()] = hostPort
	cfg.Logger.Info("connected via mDNS", "instance", e.ServiceInstanceName(), "addr", hostPort)
}

// reconcile drops entries from the connected map whose host:port is no longer
// an active ("device" state) entry in `adb devices`. "offline" entries don't
// count — those are stale endpoints left behind after the phone rotated ports.
// We also run `adb disconnect` on any offline entries to keep the device list
// tidy. Dropping from the map lets the next mDNS announcement re-trigger a
// fresh connect.
func reconcile(ctx context.Context, cfg Config, connected map[string]string) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	devices, err := cfg.adbDevicesFn(ctx)
	if err != nil {
		cfg.Logger.Warn("adb devices failed during reconcile", "err", err)
		return
	}

	// Sweep: clean up any offline wifi-debug entries regardless of what's in our map.
	for _, d := range devices {
		if d.State != "offline" {
			continue
		}
		// Only disconnect LAN-style serials (contain ":"); leave USB serials alone.
		host, port, ok := splitHostPort(d.Serial)
		if !ok {
			continue
		}
		cfg.Logger.Info("disconnecting stale offline endpoint", "addr", d.Serial)
		if err := cfg.adbDisconnectFn(ctx, host, port); err != nil {
			cfg.Logger.Debug("adb disconnect failed (ignored)", "addr", d.Serial, "err", err)
		}
	}

	if len(connected) == 0 {
		return
	}

	// Only entries currently in state "device" count as active.
	active := make(map[string]bool, len(devices))
	for _, d := range devices {
		if d.State == "device" {
			active[d.Serial] = true
		}
	}

	for instance, hostPort := range connected {
		if !active[hostPort] {
			cfg.Logger.Info("device no longer active in adb devices, dropping from connected map", "instance", instance, "addr", hostPort)
			delete(connected, instance)
		}
	}
}

// splitHostPort parses a "host:port" string (as `adb devices` prints wifi-debug
// serials). Returns ok=false for serials without a ":" (USB serials, emulator IDs).
func splitHostPort(s string) (host string, port int, ok bool) {
	// Find last ":" to allow IPv6 bracketed forms.
	idx := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			idx = i
			break
		}
	}
	if idx <= 0 || idx == len(s)-1 {
		return "", 0, false
	}
	p := 0
	for i := idx + 1; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return "", 0, false
		}
		p = p*10 + int(c-'0')
	}
	return s[:idx], p, true
}

// defaultBrowse uses the real zeroconf library.
func defaultBrowse(ctx context.Context, entries chan<- *zeroconf.ServiceEntry) error {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("zeroconf resolver: %w", err)
	}
	return resolver.Browse(ctx, "_adb-tls-connect._tcp", "local.", entries)
}

// defaultAdbConnect delegates to the internal adb package.
func defaultAdbConnect(ctx context.Context, host string, port int) error {
	r, err := adb.Connect(ctx, host, port)
	if err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("adb connect: %s", r.Stderr)
	}
	return nil
}

// defaultAdbDisconnect runs `adb disconnect <host>:<port>`. Errors are best-effort;
// `adb disconnect` of a serial that isn't connected returns non-zero, which is fine.
func defaultAdbDisconnect(ctx context.Context, host string, port int) error {
	r, err := adb.Disconnect(ctx, host, port)
	if err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("adb disconnect: %s", r.Stderr)
	}
	return nil
}

// defaultAdbDevices delegates to the internal adb package.
func defaultAdbDevices(ctx context.Context) ([]adbDevice, error) {
	devs, err := adb.Devices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]adbDevice, len(devs))
	for i, d := range devs {
		out[i] = adbDevice{Serial: d.Serial, State: d.State}
	}
	return out, nil
}
