// Package wsserver implements the phone-facing WebSocket server over Tailscale.
package wsserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/remote/protocol"
)

const connectTimeout = 20 * time.Second

// Config wires dependencies into Server.
type Config struct {
	Store    *statestore.Store
	PSK      string
	BindHost string
	BindPort int
	Logger   *slog.Logger
}

// BusinessError carries a machine-readable code ("phone_offline", "connect_timeout", …).
type BusinessError struct {
	Code    string
	Message string
}

func (e *BusinessError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// ErrorCode satisfies the interface{ ErrorCode() string } contract used by the
// IPC layer to surface machine-readable error codes to CLI callers.
func (e *BusinessError) ErrorCode() string { return e.Code }

// Server is a Tailscale-bound WS listener with per-phone connection bookkeeping.
type Server struct {
	cfg     Config
	ln      net.Listener
	httpSrv *http.Server

	mu      sync.Mutex
	byNick  map[string]*conn
	pending map[string]*pending
}

type conn struct {
	ws       *websocket.Conn
	nickname string
}

type pending struct {
	done chan pendingResult
}

type pendingResult struct {
	ready *protocol.ConnectReady
	err   error
}

func New(cfg Config) (*Server, error) {
	if cfg.Store == nil || cfg.PSK == "" || cfg.Logger == nil {
		return nil, errors.New("wsserver: missing required Config fields")
	}
	return &Server{
		cfg:     cfg,
		byNick:  map[string]*conn{},
		pending: map[string]*pending{},
	}, nil
}

// Start listens on BindHost:BindPort (0 = random) and returns the chosen port.
func (s *Server) Start() (int, error) {
	addr := net.JoinHostPort(s.cfg.BindHost, strconv.Itoa(s.cfg.BindPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, err
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)
	s.httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// Stop tears down the listener and rejects any pending connects.
func (s *Server) Stop() error {
	s.mu.Lock()
	for _, p := range s.pending {
		p.done <- pendingResult{err: &BusinessError{Code: "server_stopping", Message: "server shutting down"}}
		close(p.done)
	}
	s.pending = map[string]*pending{}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// OnlineNicknames returns the nicknames currently connected.
func (s *Server) OnlineNicknames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.byNick))
	for n := range s.byNick {
		out = append(out, n)
	}
	return out
}

// RequestConnect pushes a prep_connect frame and waits up to connectTimeout for
// a connect_ready OR an error frame from the same nickname.
func (s *Server) RequestConnect(ctx context.Context, nickname string, requestPair bool) (*protocol.ConnectReady, error) {
	s.mu.Lock()
	c, ok := s.byNick[nickname]
	if !ok {
		s.mu.Unlock()
		return nil, &BusinessError{Code: "phone_offline", Message: "phone not connected: " + nickname}
	}
	p := &pending{done: make(chan pendingResult, 1)}
	s.pending[nickname] = p
	s.mu.Unlock()

	msg, err := protocol.Build(&protocol.PrepConnect{RequestPair: requestPair})
	if err != nil {
		return nil, err
	}
	wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	if err := c.ws.Write(wctx, websocket.MessageText, msg); err != nil {
		cancel()
		return nil, err
	}
	cancel()

	select {
	case r := <-p.done:
		return r.ready, r.err
	case <-time.After(connectTimeout):
		s.mu.Lock()
		delete(s.pending, nickname)
		s.mu.Unlock()
		return nil, &BusinessError{Code: "connect_timeout", Message: "timeout waiting for connect_ready: " + nickname}
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	ctx := r.Context()
	// First frame must be hello.
	_, raw, err := ws.Read(ctx)
	if err != nil {
		return
	}
	f, err := protocol.Parse(raw)
	if err != nil {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "bad_frame", Message: err.Error()})
		return
	}
	h, ok := f.(*protocol.Hello)
	if !ok {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "auth_required", Message: "hello required first"})
		return
	}
	if !pskEquals(h.PSK, s.cfg.PSK) {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "auth_failed", Message: "bad psk"})
		return
	}
	phone, err := s.cfg.Store.GetPhone(h.Nickname)
	if err != nil || phone == nil {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "unknown_phone", Message: "phone not enrolled"})
		return
	}
	s.mu.Lock()
	s.byNick[h.Nickname] = &conn{ws: ws, nickname: h.Nickname}
	s.mu.Unlock()
	_ = s.cfg.Store.RecordPhoneSeen(h.Nickname)
	s.cfg.Logger.Info("phone online", "nickname", h.Nickname)
	defer func() {
		s.mu.Lock()
		if cur := s.byNick[h.Nickname]; cur != nil && cur.ws == ws {
			delete(s.byNick, h.Nickname)
		}
		s.mu.Unlock()
		s.cfg.Logger.Info("phone offline", "nickname", h.Nickname)
	}()
	s.send(ctx, ws, &protocol.Ack{})

	// Authed loop.
	for {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			return
		}
		frame, err := protocol.Parse(raw)
		if err != nil {
			s.send(ctx, ws, &protocol.ErrorFrame{Code: "bad_frame", Message: err.Error()})
			continue
		}
		switch v := frame.(type) {
		case *protocol.ToggleState:
			s.cfg.Logger.Info("toggle", "nickname", h.Nickname, "on", v.On)
		case *protocol.ConnectReady:
			s.mu.Lock()
			p := s.pending[h.Nickname]
			delete(s.pending, h.Nickname)
			s.mu.Unlock()
			if p != nil {
				p.done <- pendingResult{ready: v}
				close(p.done)
			}
		case *protocol.ErrorFrame:
			s.cfg.Logger.Warn("phone error", "code", v.Code, "message", v.Message)
			s.mu.Lock()
			p := s.pending[h.Nickname]
			delete(s.pending, h.Nickname)
			s.mu.Unlock()
			if p != nil {
				p.done <- pendingResult{err: &BusinessError{Code: v.Code, Message: v.Message}}
				close(p.done)
			}
		default:
			s.send(ctx, ws, &protocol.ErrorFrame{Code: "bad_frame", Message: "unexpected op"})
		}
	}
}

func (s *Server) send(ctx context.Context, ws *websocket.Conn, f protocol.Frame) {
	b, err := protocol.Build(f)
	if err != nil {
		s.cfg.Logger.Warn("build frame", "err", err)
		return
	}
	wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = ws.Write(wctx, websocket.MessageText, b)
}

func pskEquals(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
