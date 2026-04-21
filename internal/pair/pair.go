// Package pair implements the same-LAN ADB wireless pairing flow.
// It generates a random service name and password, encodes them as a Wi-Fi QR
// payload, serves the QR via an in-process HTTP server (or renders it to the
// terminal in headless mode), browses mDNS for the Android pairing service,
// and runs `adb pair` followed by `adb connect`.
package pair

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/skip2/go-qrcode"

	"github.com/premex-ab/adb-connect/internal/adb"
)

const (
	defaultTimeout       = 3 * time.Minute
	connectBrowseTimeout = 30 * time.Second
	servicePairingType   = "_adb-tls-pairing._tcp"
	serviceConnectType   = "_adb-tls-connect._tcp"
	mdnsDomain           = "local."
	qrPNGSize            = 512
)

// Config controls the behaviour of Run.
type Config struct {
	// Open, if non-nil, is called with the local URL of the QR page and should
	// open that URL in a browser.  If nil, the QR is rendered as ASCII to the
	// logger instead (terminal-only mode).
	Open func(url string) error

	// Timeout caps the total flow (default 3 minutes).
	Timeout time.Duration

	// Logger receives progress messages.  nil falls back to slog.Default().
	Logger *slog.Logger

	// --- test-only seams (unexported; prod code leaves them nil) ---

	// browsePair, if non-nil, replaces the real mDNS browse for the pairing
	// service.  Must close the returned channel (or return an error) when the
	// context is cancelled.
	browsePair func(ctx context.Context, serviceName string) (<-chan browseResult, error)

	// browseConnect, if non-nil, replaces the real mDNS browse for the connect
	// service.
	browseConnect func(ctx context.Context, peerHost string) (<-chan browseResult, error)

	// runAdbPair, if non-nil, replaces the real adb.Pair call.
	runAdbPair func(ctx context.Context, host string, port int, code string) error

	// runAdbConnect, if non-nil, replaces the real adb.Connect call.
	runAdbConnect func(ctx context.Context, host string, port int) error
}

// browseResult carries a single resolved mDNS service entry.
type browseResult struct {
	Host string
	Port int
}

// Run executes the same-LAN pair flow. Returns the final `adb connect` address
// on success (e.g. "10.0.1.136:44321"), or an error.
// Config.Open controls whether the browser is launched; if nil, the browser is not opened
// (terminal-only mode — QR is rendered to the logger via go-qrcode's ToString).
// Config.Timeout caps the total flow (default 3 minutes).
func Run(ctx context.Context, cfg Config) (string, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Generate credentials.
	serviceName, err := randomServiceName()
	if err != nil {
		return "", fmt.Errorf("pair: generate service name: %w", err)
	}
	password, err := randomPassword()
	if err != nil {
		return "", fmt.Errorf("pair: generate password: %w", err)
	}
	qrPayload := fmt.Sprintf("WIFI:T:ADB;S:%s;P:%s;;", serviceName, password)

	log.Info("On your Android device: Settings -> Developer options -> Wireless debugging -> Pair device with QR code")

	if cfg.Open != nil {
		// Serve mode: generate PNG, start HTTP server, open browser.
		url, shutdownHTTP, err := serveQR(qrPayload, serviceName, log)
		if err != nil {
			return "", fmt.Errorf("pair: start QR HTTP server: %w", err)
		}
		defer func() { _ = shutdownHTTP(context.Background()) }()

		if openErr := cfg.Open(url); openErr != nil {
			log.Warn("pair: could not open browser (continuing anyway)", "err", openErr)
		}
		log.Info("pair: QR page served", "url", url)
	} else {
		// Terminal mode: render ASCII QR.
		qr, err := qrcode.New(qrPayload, qrcode.Low)
		if err != nil {
			return "", fmt.Errorf("pair: generate QR: %w", err)
		}
		ascii := qr.ToString(false)
		log.Info("pair: scan the QR code below\n" + ascii)
	}

	log.Info("pair: waiting for device to advertise pairing service...", "service", serviceName)

	// --- Browse for pairing service ---
	pairCh, err := browseForPair(ctx, cfg, serviceName)
	if err != nil {
		return "", fmt.Errorf("pair: start mDNS browse (pair): %w", err)
	}

	var pairHost string
	var pairPort int
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("pair: timed out waiting for device to advertise pairing service: %w", ctx.Err())
	case entry, ok := <-pairCh:
		if !ok {
			return "", errors.New("pair: mDNS browse channel closed before match")
		}
		pairHost = entry.Host
		pairPort = entry.Port
	}

	log.Info("pair: found pairing service", "host", pairHost, "port", pairPort)

	// --- adb pair ---
	if err := runPair(ctx, cfg, pairHost, pairPort, password); err != nil {
		return "", fmt.Errorf("adb pair %s:%d: %w", pairHost, pairPort, err)
	}
	log.Info("pair: paired successfully; waiting for connect service...")

	// --- Browse for connect service (shorter timeout) ---
	connectCtx, connectCancel := context.WithTimeout(ctx, connectBrowseTimeout)
	defer connectCancel()

	connectCh, err := browseForConnect(connectCtx, cfg, pairHost)
	if err != nil {
		return "", fmt.Errorf("pair: start mDNS browse (connect): %w", err)
	}

	var connHost string
	var connPort int
	select {
	case <-connectCtx.Done():
		return "", fmt.Errorf("pair: timed out waiting for connect service after pairing: %w", connectCtx.Err())
	case entry, ok := <-connectCh:
		if !ok {
			return "", errors.New("pair: mDNS connect browse channel closed before match")
		}
		connHost = entry.Host
		connPort = entry.Port
	}

	log.Info("pair: found connect service", "host", connHost, "port", connPort)

	// --- adb connect ---
	if err := runConnect(ctx, cfg, connHost, connPort); err != nil {
		return "", fmt.Errorf("adb connect %s:%d: %w", connHost, connPort, err)
	}

	addr := fmt.Sprintf("%s:%d", connHost, connPort)
	log.Info("pair: connected", "addr", addr)
	return addr, nil
}

// --- browse helpers ---

func browseForPair(ctx context.Context, cfg Config, serviceName string) (<-chan browseResult, error) {
	if cfg.browsePair != nil {
		return cfg.browsePair(ctx, serviceName)
	}
	return realBrowse(ctx, servicePairingType, func(entry *zeroconf.ServiceEntry) (browseResult, bool) {
		if entry.Instance != serviceName {
			return browseResult{}, false
		}
		h := pickIPv4(entry)
		if h == "" {
			return browseResult{}, false
		}
		return browseResult{Host: h, Port: entry.Port}, true
	})
}

func browseForConnect(ctx context.Context, cfg Config, peerHost string) (<-chan browseResult, error) {
	if cfg.browseConnect != nil {
		return cfg.browseConnect(ctx, peerHost)
	}
	return realBrowse(ctx, serviceConnectType, func(entry *zeroconf.ServiceEntry) (browseResult, bool) {
		h := pickIPv4(entry)
		if h == peerHost {
			return browseResult{Host: h, Port: entry.Port}, true
		}
		return browseResult{}, false
	})
}

// realBrowse launches a zeroconf Browse and feeds matching entries into the
// returned channel.  The channel is closed when the context is done.
func realBrowse(
	ctx context.Context,
	serviceType string,
	match func(*zeroconf.ServiceEntry) (browseResult, bool),
) (<-chan browseResult, error) {
	resolver, err := zeroconf.NewResolver()
	if err != nil {
		return nil, err
	}

	raw := make(chan *zeroconf.ServiceEntry, 8)
	if err := resolver.Browse(ctx, serviceType, mdnsDomain, raw); err != nil {
		return nil, err
	}

	out := make(chan browseResult, 1)
	go func() {
		defer close(out)
		for entry := range raw {
			if r, ok := match(entry); ok {
				select {
				case out <- r:
				default:
				}
				return
			}
		}
	}()
	return out, nil
}

// --- adb helpers ---

func runPair(ctx context.Context, cfg Config, host string, port int, code string) error {
	if cfg.runAdbPair != nil {
		return cfg.runAdbPair(ctx, host, port, code)
	}
	r, err := adb.Pair(ctx, host, port, code)
	if err != nil {
		return err
	}
	if !r.OK {
		msg := r.Stderr
		if msg == "" {
			msg = r.Stdout
		}
		return fmt.Errorf("exit %d: %s", r.Code, msg)
	}
	return nil
}

func runConnect(ctx context.Context, cfg Config, host string, port int) error {
	if cfg.runAdbConnect != nil {
		return cfg.runAdbConnect(ctx, host, port)
	}
	r, err := adb.Connect(ctx, host, port)
	if err != nil {
		return err
	}
	if !r.OK {
		msg := r.Stderr
		if msg == "" {
			msg = r.Stdout
		}
		return fmt.Errorf("exit %d: %s", r.Code, msg)
	}
	return nil
}

// --- HTTP QR server ---

func serveQR(qrPayload, serviceName string, log *slog.Logger) (url string, shutdown func(context.Context) error, err error) {
	png, err := qrcode.Encode(qrPayload, qrcode.Low, qrPNGSize)
	if err != nil {
		return "", nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	html := buildHTML(png)

	mux := http.NewServeMux()
	mux.HandleFunc("/qr.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()

	addr := ln.Addr().String()
	return fmt.Sprintf("http://%s/", addr), srv.Shutdown, nil
}

func buildHTML(png []byte) []byte {
	// Embed the QR PNG as a base64 data URL so the page works even after the
	// HTTP server is shut down (browser caches the page).
	encoded := make([]byte, len(png)*4/3+4)
	n := encodeBase64(encoded, png)
	dataURL := "data:image/png;base64," + string(encoded[:n])

	page := `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>ADB Wi-Fi Pairing</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 32px 16px;
         background: #f5f5f7; color: #1a1a1a; min-height: 100vh; box-sizing: border-box;
         display: flex; flex-direction: column; align-items: center; gap: 8px; }
  @media (prefers-color-scheme: dark) { body { background: #1a1a1c; color: #e8e8ea; } }
  h1 { margin: 0; font-size: 22px; font-weight: 600; text-align: center; }
  p { color: inherit; opacity: .75; max-width: 480px; margin: 4px auto; line-height: 1.5; text-align: center; }
  #slot { width: min(70vw, 380px); aspect-ratio: 1/1; background: white;
          padding: 16px; border-radius: 16px; box-shadow: 0 8px 24px rgba(0,0,0,.08); margin: 16px 0;
          display: flex; align-items: center; justify-content: center; box-sizing: border-box; }
  @media (prefers-color-scheme: dark) { #slot { background: #2a2a2c; } }
  #slot img { width: 100%; height: 100%; display: block; }
</style>
</head>
<body>
  <h1>Pair Android device over Wi-Fi</h1>
  <p>Open <b>Settings &#8594; Developer options &#8594; Wireless debugging &#8594; Pair device with QR code</b> on your phone, then scan the code below.</p>
  <div id="slot"><img src="` + dataURL + `" alt="Pairing QR code"/></div>
  <p>Waiting for device — this page will update automatically.</p>
</body>
</html>`
	return []byte(page)
}

// encodeBase64 encodes src into dst using standard base64 and returns the
// number of bytes written.  We use encoding/base64 indirectly by calling the
// stdlib Encoder.
func encodeBase64(dst, src []byte) int {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	di, si := 0, 0
	n := (len(src) / 3) * 3
	for si < n {
		val := uint(src[si+0])<<16 | uint(src[si+1])<<8 | uint(src[si+2])
		dst[di+0] = table[val>>18&0x3F]
		dst[di+1] = table[val>>12&0x3F]
		dst[di+2] = table[val>>6&0x3F]
		dst[di+3] = table[val>>0&0x3F]
		si += 3
		di += 4
	}
	rem := len(src) - si
	if rem == 2 {
		val := uint(src[si+0])<<16 | uint(src[si+1])<<8
		dst[di+0] = table[val>>18&0x3F]
		dst[di+1] = table[val>>12&0x3F]
		dst[di+2] = table[val>>6&0x3F]
		dst[di+3] = '='
		di += 4
	} else if rem == 1 {
		val := uint(src[si+0]) << 16
		dst[di+0] = table[val>>18&0x3F]
		dst[di+1] = table[val>>12&0x3F]
		dst[di+2] = '='
		dst[di+3] = '='
		di += 4
	}
	return di
}

// --- credential generation ---

func randomServiceName() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ADB_WIFI_" + hex.EncodeToString(b), nil
}

func randomPassword() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- misc helpers ---

func pickIPv4(entry *zeroconf.ServiceEntry) string {
	for _, ip := range entry.AddrIPv4 {
		if ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}

// OpenBrowser opens url in the system default browser.
// It is a best-effort operation; errors are returned but the caller need not
// treat them as fatal.
func OpenBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", "", url}
	default: // linux and others
		cmd = "xdg-open"
		args = []string{url}
	}
	c := exec.Command(cmd, args...)
	return c.Start()
}
