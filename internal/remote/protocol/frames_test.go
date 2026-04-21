package protocol_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/protocol"
)

func TestParse_AcceptsHello(t *testing.T) {
	raw := []byte(`{"op":"hello","nickname":"alpha","psk":"YWJj","app_version":"0.1.0"}`)
	f, err := protocol.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	h, ok := f.(*protocol.Hello)
	if !ok {
		t.Fatalf("got %T, want *Hello", f)
	}
	if h.Nickname != "alpha" || h.PSK != "YWJj" {
		t.Fatalf("hello fields: %+v", h)
	}
}

func TestParse_RejectsUnknownOp(t *testing.T) {
	_, err := protocol.Parse([]byte(`{"op":"nonsense"}`))
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Code != "bad_frame" {
		t.Fatalf("want bad_frame ProtocolError, got %v", err)
	}
}

func TestParse_RejectsMissingFields(t *testing.T) {
	for _, raw := range []string{
		`{"op":"hello"}`,
		`{"op":"connect_ready"}`,
		`not json`,
	} {
		if _, err := protocol.Parse([]byte(raw)); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestBuild_PrepConnectAndError(t *testing.T) {
	b, err := protocol.Build(&protocol.PrepConnect{RequestPair: true})
	if err != nil {
		t.Fatalf("build prep: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["op"] != "prep_connect" || m["request_pair"] != true {
		t.Fatalf("prep round-trip: %v", m)
	}
	b, err = protocol.Build(&protocol.ErrorFrame{Code: "auth_failed", Message: "bad psk"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["code"] != "auth_failed" {
		t.Fatalf("error code: %v", m)
	}
}
