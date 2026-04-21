package ipcserver

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
)

// Request sends a line-delimited JSON request to the daemon's IPC socket and returns the response map.
func Request(req map[string]any) (map[string]any, error) {
	c, err := net.DialTimeout("unix", paths.IPCSocketPath(), 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		if sc.Err() != nil {
			return nil, sc.Err()
		}
		return nil, errors.New("empty ipc response")
	}
	var resp map[string]any
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
