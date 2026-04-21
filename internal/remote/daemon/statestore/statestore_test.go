package statestore_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func open(t *testing.T) *statestore.Store {
	t.Helper()
	testutil.TempHome(t)
	if err := os.MkdirAll(paths.ConfigDir(), 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	s, err := statestore.Open(paths.DBPath())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestServerConfigRoundTrip(t *testing.T) {
	s := open(t)
	if cfg, err := s.ServerConfig(); err != nil || cfg != nil {
		t.Fatalf("expected nil cfg, got %v err %v", cfg, err)
	}
	want := &statestore.ServerConfig{PSK: []byte("abc"), WSPort: 34567, TailscaleHost: "mac.ts.net"}
	if err := s.SetServerConfig(want); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.ServerConfig()
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if !bytes.Equal(got.PSK, want.PSK) || got.WSPort != want.WSPort || got.TailscaleHost != want.TailscaleHost {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUpsertPhoneAndLastSeen(t *testing.T) {
	s := open(t)
	if err := s.UpsertPhone(statestore.Phone{Nickname: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPhone(statestore.Phone{Nickname: "beta", Paired: true, ADBFingerprint: "beef"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPhoneSeen("alpha"); err != nil {
		t.Fatal(err)
	}
	phones, err := s.ListPhones()
	if err != nil || len(phones) != 2 {
		t.Fatalf("listed %d phones: %v", len(phones), err)
	}
	for _, p := range phones {
		if p.Nickname == "alpha" && p.LastWSSeen.IsZero() {
			t.Fatalf("alpha last seen zero")
		}
		if p.Nickname == "beta" && !p.Paired {
			t.Fatalf("beta not paired")
		}
	}
	time.Sleep(10 * time.Millisecond) // avoid identical timestamps across tests
}

func TestGetPhoneReturnsNilForUnknown(t *testing.T) {
	s := open(t)
	p, err := s.GetPhone("nobody")
	if err != nil || p != nil {
		t.Fatalf("got %v err %v", p, err)
	}
}
