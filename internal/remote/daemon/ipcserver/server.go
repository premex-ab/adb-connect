// Package ipcserver runs a Unix-domain line-delimited JSON RPC over a 0600 socket.
// It is the transport between the adb-connect CLI subcommands (connect/status) and the daemon.
package ipcserver

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
)

// Handler processes a request payload and returns a response payload or an error.
type Handler func(req map[string]any) (map[string]any, error)

type Server struct {
	handlers map[string]Handler
	ln       net.Listener
	mu       sync.Mutex
}

func New(handlers map[string]Handler) (*Server, error) {
	if handlers == nil {
		return nil, errors.New("ipcserver: nil handlers")
	}
	return &Server{handlers: handlers}, nil
}

func (s *Server) Start() error {
	if err := os.MkdirAll(paths.ConfigDir(), 0o700); err != nil {
		return err
	}
	sock := paths.IPCSocketPath()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	_ = os.Chmod(sock, 0o600)
	s.ln = ln
	go s.loop()
	return nil
}

func (s *Server) loop() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *Server) Stop() {
	if s.ln != nil {
		_ = s.ln.Close()
		_ = os.Remove(paths.IPCSocketPath())
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		return
	}
	var req map[string]any
	if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
		writeResp(c, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	op, _ := req["op"].(string)
	h, ok := s.handlers[op]
	if !ok {
		writeResp(c, map[string]any{"ok": false, "error": fmt.Sprintf("unknown op: %s", op)})
		return
	}
	resp, err := h(req)
	if err != nil {
		writeResp(c, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if resp == nil {
		resp = map[string]any{}
	}
	resp["ok"] = true
	writeResp(c, resp)
}

func writeResp(c net.Conn, m map[string]any) {
	b, _ := json.Marshal(m)
	_, _ = c.Write(append(b, '\n'))
}
