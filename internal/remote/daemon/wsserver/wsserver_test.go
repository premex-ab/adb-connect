package wsserver_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/wsserver"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func newStore(t *testing.T) *statestore.Store {
	t.Helper()
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)
	s, err := statestore.Open(paths.DBPath())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newServer(t *testing.T, store *statestore.Store, psk string) (*wsserver.Server, int) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv, err := wsserver.New(wsserver.Config{Store: store, PSK: psk, BindHost: "127.0.0.1", BindPort: 0, Logger: log})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv, port
}

func dial(t *testing.T, port int) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), "ws://127.0.0.1:"+itoa(port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func itoa(n int) string { return strconvItoa(n) }

// avoid pulling strconv in tests for this one call:
func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 5)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestHelloWrongPSKReturnsAuthFailed(t *testing.T) {
	s := newStore(t)
	_ = s.UpsertPhone(statestore.Phone{Nickname: "alpha"})
	_, port := newServer(t, s, "correct")
	c := dial(t, port)
	defer c.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"op":"hello","nickname":"alpha","psk":"wrong","app_version":"t"}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"code":"auth_failed"`; !containsJSON(data, want) {
		t.Fatalf("got %s, want %s", data, want)
	}
}

func TestHelloSuccessRecordsLastSeen(t *testing.T) {
	s := newStore(t)
	_ = s.UpsertPhone(statestore.Phone{Nickname: "alpha"})
	_, port := newServer(t, s, "correct")
	c := dial(t, port)
	defer c.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":"hello","nickname":"alpha","psk":"correct","app_version":"t"}`))
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsJSON(data, `"op":"ack"`) {
		t.Fatalf("want ack, got %s", data)
	}
	p, _ := s.GetPhone("alpha")
	if p.LastWSSeen.IsZero() {
		t.Fatalf("last_seen not recorded")
	}
}

func TestRequestConnectRejectsOfflinePhone(t *testing.T) {
	s := newStore(t)
	_ = s.UpsertPhone(statestore.Phone{Nickname: "alpha"})
	srv, _ := newServer(t, s, "psk")
	_, err := srv.RequestConnect(context.Background(), "alpha", true)
	var berr *wsserver.BusinessError
	if !errors.As(err, &berr) || berr.Code != "phone_offline" {
		t.Fatalf("want phone_offline, got %v", err)
	}
}

func containsJSON(b []byte, sub string) bool {
	s := string(b)
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
