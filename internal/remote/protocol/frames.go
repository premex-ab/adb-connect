// Package protocol defines the JSON frame shapes exchanged over the adb-connect WebSocket.
// This is the canonical contract — the Kotlin WsProtocol in android-app/ must stay in sync.
package protocol

import (
	"encoding/json"
	"fmt"
)

// Error is returned by Parse/Build for malformed frames. It carries an on-wire error code
// suitable for sending as the code field of an error frame.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("protocol %s: %s", e.Code, e.Message) }

// Frame marks types that are valid WS frames.
type Frame interface{ isFrame() }

type Hello struct {
	Nickname   string `json:"nickname"`
	PSK        string `json:"psk"`
	AppVersion string `json:"app_version"`
}

type ToggleState struct {
	On bool `json:"on"`
}

type PrepConnect struct {
	RequestPair bool `json:"request_pair"`
}

type ConnectReady struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	PairCode string `json:"pair_code,omitempty"`
}

type Ack struct{}

type ErrorFrame struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (*Hello) isFrame()        {}
func (*ToggleState) isFrame()  {}
func (*PrepConnect) isFrame()  {}
func (*ConnectReady) isFrame() {}
func (*Ack) isFrame()          {}
func (*ErrorFrame) isFrame()   {}

// Parse validates a raw JSON frame and returns the typed representation.
func Parse(raw []byte) (Frame, error) {
	var peek struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		return nil, &Error{Code: "bad_frame", Message: "invalid JSON"}
	}
	if peek.Op == "" {
		return nil, &Error{Code: "bad_frame", Message: "missing op"}
	}
	switch peek.Op {
	case "hello":
		var h Hello
		if err := json.Unmarshal(raw, &h); err != nil {
			return nil, &Error{Code: "bad_frame", Message: err.Error()}
		}
		if h.Nickname == "" || h.PSK == "" {
			return nil, &Error{Code: "bad_frame", Message: "hello missing nickname or psk"}
		}
		return &h, nil
	case "toggle_state":
		var t struct {
			Op string `json:"op"`
			On *bool  `json:"on"`
		}
		if err := json.Unmarshal(raw, &t); err != nil || t.On == nil {
			return nil, &Error{Code: "bad_frame", Message: "toggle_state missing on"}
		}
		return &ToggleState{On: *t.On}, nil
	case "prep_connect":
		var p struct {
			Op          string `json:"op"`
			RequestPair *bool  `json:"request_pair"`
		}
		if err := json.Unmarshal(raw, &p); err != nil || p.RequestPair == nil {
			return nil, &Error{Code: "bad_frame", Message: "prep_connect missing request_pair"}
		}
		return &PrepConnect{RequestPair: *p.RequestPair}, nil
	case "connect_ready":
		var c ConnectReady
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, &Error{Code: "bad_frame", Message: err.Error()}
		}
		if c.IP == "" || c.Port == 0 {
			return nil, &Error{Code: "bad_frame", Message: "connect_ready missing ip/port"}
		}
		return &c, nil
	case "ack":
		return &Ack{}, nil
	case "error":
		var e ErrorFrame
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, &Error{Code: "bad_frame", Message: err.Error()}
		}
		return &e, nil
	default:
		return nil, &Error{Code: "bad_frame", Message: "unknown op: " + peek.Op}
	}
}

// Build serializes a Frame for the wire. The op is derived from the concrete type.
func Build(f Frame) ([]byte, error) {
	switch v := f.(type) {
	case *Hello:
		return json.Marshal(struct {
			Op string `json:"op"`
			*Hello
		}{"hello", v})
	case *ToggleState:
		return json.Marshal(struct {
			Op string `json:"op"`
			*ToggleState
		}{"toggle_state", v})
	case *PrepConnect:
		return json.Marshal(struct {
			Op string `json:"op"`
			*PrepConnect
		}{"prep_connect", v})
	case *ConnectReady:
		return json.Marshal(struct {
			Op string `json:"op"`
			*ConnectReady
		}{"connect_ready", v})
	case *Ack:
		return []byte(`{"op":"ack"}`), nil
	case *ErrorFrame:
		return json.Marshal(struct {
			Op string `json:"op"`
			*ErrorFrame
		}{"error", v})
	default:
		return nil, &Error{Code: "bad_frame", Message: fmt.Sprintf("unknown frame type %T", f)}
	}
}
