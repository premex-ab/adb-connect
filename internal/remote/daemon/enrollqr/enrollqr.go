// Package enrollqr renders the PSK-and-server-host QR over a loopback HTTP server,
// intended to be opened in the user's browser during `adb-connect remote setup`.
package enrollqr

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/skip2/go-qrcode"
)

type Payload struct {
	Version int    `json:"v"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	PSK     string `json:"psk"`
}

// Serve starts an HTTP server on 127.0.0.1:0 serving a page with the QR for payload.
// Caller must call the returned shutdown() when done (typically after IPC status reports the phone online).
func Serve(payload Payload) (url string, shutdown func(ctx context.Context) error, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		_ = ln.Close()
		return "", nil, err
	}
	png, err := qrcode.Encode(string(body), qrcode.Medium, 512)
	if err != nil {
		_ = ln.Close()
		return "", nil, err
	}
	srv := &http.Server{
		Handler:           handler(png, payload),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	return fmt.Sprintf("http://%s", ln.Addr()), srv.Shutdown, nil
}

func handler(png []byte, p Payload) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/qr.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!doctype html><html><head><meta charset=utf-8><title>Premex ADB-gate – enroll</title>
<style>body{font-family:system-ui,sans-serif;background:#111;color:#eee;display:flex;flex-direction:column;align-items:center;padding:2em}
img{background:#fff;padding:1em;border-radius:8px;max-width:80vw}
h1{color:#ff6b00;font-weight:600}</style></head>
<body><h1>Premex ADB-gate</h1><p>Open the Premex ADB-gate app on your phone and scan this QR.</p>
<img src="/qr.png"><p style="opacity:.7;font-size:.9em">server: %s:%d</p></body></html>`, p.Host, p.Port)
		}
	})
}
