package ipcserver_test

import (
	"os"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/ipcserver"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestStatusAndUnknownOp(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)
	srv, err := ipcserver.New(map[string]ipcserver.Handler{
		"status": func(req map[string]any) (map[string]any, error) {
			return map[string]any{"phones": []any{map[string]any{"nickname": "alpha"}}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	r, err := ipcserver.Request(map[string]any{"op": "status"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := r["ok"].(bool); !ok {
		t.Fatalf("not ok: %v", r)
	}
	r, err = ipcserver.Request(map[string]any{"op": "bogus"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := r["ok"].(bool); ok {
		t.Fatalf("expected failure: %v", r)
	}
}
