package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/ipcserver"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestDaemon_StatusIPCRoundTrip(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)

	// Seed config so runDaemon doesn't bail early.
	store, err := statestore.Open(paths.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	_ = store.SetServerConfig(&statestore.ServerConfig{PSK: []byte("psk"), WSPort: 0, TailscaleHost: "test.ts.net"})
	store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- runDaemon(ctx) }()

	// Poll for IPC socket readiness.
	deadline := time.Now().Add(5 * time.Second)
	var r map[string]any
	for {
		r, err = ipcserver.Request(map[string]any{"op": "status"})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ipc request: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if ok, _ := r["ok"].(bool); !ok {
		t.Fatalf("status not ok: %+v", r)
	}
	phones, _ := r["phones"].([]any)
	if len(phones) != 0 {
		t.Fatalf("expected empty phones, got %v", phones)
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not exit on context cancel")
	}
}
