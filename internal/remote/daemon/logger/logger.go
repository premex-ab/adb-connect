// Package logger wraps log/slog with a file-rotating handler sized for the daemon's log volume.
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
)

const maxBytes = 10 * 1024 * 1024 // 10 MB

type rotatingWriter struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		w.f = f
	}
	st, err := w.f.Stat()
	if err == nil && st.Size() >= maxBytes {
		_ = w.f.Close()
		_ = os.Rename(w.path, w.path+".1")
		w.f, err = os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
	}
	return w.f.Write(p)
}

// New returns a slog.Logger writing to both stderr and the rotating file at paths.LogPath().
func New() *slog.Logger {
	w := &rotatingWriter{path: paths.LogPath()}
	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, w), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
