// Package bootstrap implements the end-to-end remote-setup flow: install Tailscale,
// bring it up, generate PSK + WS port config, install the daemon as a user service,
// enroll a phone via QR code, and poll IPC until the phone comes online.
//
// This is a port of the Node.js bootstrap.js from the Claude-plugin precursor
// (plugins/adb-connect/scripts/bootstrap/bootstrap.js), preserving the same
// sequencing, prompts, and error messages.
package bootstrap

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/premex-ab/adb-connect/internal/apk"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/enrollqr"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/remote/service"
	"github.com/premex-ab/adb-connect/internal/tailscale"
)

const packageID = "se.premex.adbgate"

var nicknameRE = regexp.MustCompile(`^[a-z0-9-]{2,32}$`)

// adbDevice mirrors adb.Device but is defined locally to keep the seam self-contained.
type adbDevice struct{ Serial, State string }

// RemoteSetupOpts configures the remote-setup orchestrator.
type RemoteSetupOpts struct {
	// Platform is runtime.GOOS when empty.
	Platform string
	// Version is the CLI version used to select the APK to download. Empty or "dev" triggers source build.
	Version string
	// FromSource forces a Gradle source build instead of APK download.
	FromSource bool
	// NonInteractive reads auth-key and nickname from Input instead of prompting on stdout.
	NonInteractive bool
	// Input is os.Stdin when nil.
	Input io.Reader
	// Output is os.Stdout when nil.
	Output io.Writer
	// OpenBrowser is called with the enrollment QR URL. When nil the URL is only printed.
	OpenBrowser func(url string) error
	// Logger receives structured log lines. When nil slog.Default() is used.
	Logger *slog.Logger

	// Test-only seams — nil uses production implementations; all external side-effects go through these.
	ensureTSInstalled func(ctx context.Context, platform string) error
	ensureTSUp        func(ctx context.Context, key string) error
	tsStatusFn        func(ctx context.Context) *tailscale.Status
	tsMagicDNSFn      func(ctx context.Context) string
	adbDevicesFn      func(ctx context.Context) ([]adbDevice, error)
	adbInstallFn      func(ctx context.Context, apkPath string) error
	adbGrantFn        func(ctx context.Context, pkg string) error
	gradleAssembleFn  func(ctx context.Context, dir string) error
	apkDownloadFn     func(version, dest string) error
	serviceInstallFn  func(opts service.InstallOpts) error
}

// RemoteSetup runs the complete remote-setup sequence.
func RemoteSetup(ctx context.Context, opts RemoteSetupOpts) error {
	if opts.Platform == "" {
		opts.Platform = runtime.GOOS
	}
	if opts.Input == nil {
		opts.Input = os.Stdin
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	// Use a single buffered reader for all prompts so sequential reads work correctly.
	inputReader := bufio.NewReader(opts.Input)
	// Wire up production implementations for any un-set seams.
	if opts.ensureTSInstalled == nil {
		opts.ensureTSInstalled = defaultEnsureTSInstalled
	}
	if opts.ensureTSUp == nil {
		opts.ensureTSUp = defaultEnsureTSUp
	}
	if opts.tsStatusFn == nil {
		opts.tsStatusFn = tailscale.GetStatus
	}
	if opts.tsMagicDNSFn == nil {
		opts.tsMagicDNSFn = tailscale.MagicDNSName
	}
	if opts.adbDevicesFn == nil {
		opts.adbDevicesFn = defaultADBDevices
	}
	if opts.adbInstallFn == nil {
		opts.adbInstallFn = defaultADBInstall
	}
	if opts.adbGrantFn == nil {
		opts.adbGrantFn = defaultADBGrant
	}
	if opts.gradleAssembleFn == nil {
		opts.gradleAssembleFn = defaultGradleAssemble
	}
	if opts.apkDownloadFn == nil {
		opts.apkDownloadFn = apk.Download
	}
	if opts.serviceInstallFn == nil {
		opts.serviceInstallFn = service.Install
	}

	out := opts.Output

	// [1/6] Tailscale installed?
	fmt.Fprintln(out, "[1/6] Checking Tailscale…")
	if err := opts.ensureTSInstalled(ctx, opts.Platform); err != nil {
		return fmt.Errorf("ensure tailscale installed: %w", err)
	}

	// [2/6] Tailscale up?
	fmt.Fprintln(out, "[2/6] Bringing Tailscale up…")
	var authKey string
	// Only prompt for an auth key if tailscale isn't already up; otherwise it's a needless
	// user interaction (and a chance to paste the wrong key).
	if s := opts.tsStatusFn(ctx); s == nil || s.BackendState != "Running" || s.Self.DNSName == "" {
		k, err := promptAuthKey(opts, inputReader)
		if err != nil {
			return err
		}
		authKey = k
	} else {
		fmt.Fprintf(out, "     already up as %s\n", strings.TrimSuffix(s.Self.DNSName, "."))
	}
	if err := opts.ensureTSUp(ctx, authKey); err != nil {
		return fmt.Errorf("tailscale up: %w", err)
	}

	// [3/6] Generate config (PSK, WS port, tailnet host).
	fmt.Fprintln(out, "[3/6] Generating daemon config…")
	cfg, err := generateConfig(ctx, opts.tsMagicDNSFn)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "     tailnet host: %s  ws port: %d\n", cfg.tailscaleHost, cfg.wsPort)

	// [4/6] Install daemon as user service.
	fmt.Fprintln(out, "[4/6] Installing daemon as user service…")
	exe, _ := os.Executable()
	if exe == "" {
		exe = "adb-connect"
	}
	if err := opts.serviceInstallFn(service.InstallOpts{BinaryPath: exe}); err != nil {
		return fmt.Errorf("install daemon service: %w", err)
	}

	// [5/6] Require exactly one phone.
	fmt.Fprintln(out, "[5/6] Checking for a phone to enroll…")
	phone, err := requireAttachedPhone(ctx, opts.adbDevicesFn)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "     using device %s\n", phone.Serial)

	// Prompt for nickname.
	nickname, err := promptNickname(opts, inputReader)
	if err != nil {
		return err
	}

	// Build / download APK.
	apkPath, err := resolveAPK(ctx, opts)
	if err != nil {
		return err
	}

	// Install on device and grant permission.
	if err := opts.adbInstallFn(ctx, apkPath); err != nil {
		return fmt.Errorf("adb install failed: %w", err)
	}
	if err := opts.adbGrantFn(ctx, packageID); err != nil {
		return fmt.Errorf("pm grant WRITE_SECURE_SETTINGS failed: %w", err)
	}

	// Upsert phone in state store.
	db, err := statestore.Open(paths.DBPath())
	if err != nil {
		return fmt.Errorf("open state store: %w", err)
	}
	if err := db.UpsertPhone(statestore.Phone{Nickname: nickname}); err != nil {
		_ = db.Close()
		return fmt.Errorf("upsert phone: %w", err)
	}
	_ = db.Close()

	// [6/6] Enrollment QR.
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Make sure the Tailscale Android app is installed on your phone and joined to your tailnet:")
	fmt.Fprintln(out, "  1. Install: https://play.google.com/store/apps/details?id=com.tailscale.ipn")
	fmt.Fprintln(out, "  2. Sign in with the same account used for the auth key you just pasted.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "[6/6] Rendering enrollment QR. Open Premex ADB-gate on the phone and scan it.")

	pskBytes, _ := hex.DecodeString(cfg.pskHex)
	payload := enrollqr.Payload{
		Version: 1,
		Host:    cfg.tailscaleHost,
		Port:    cfg.wsPort,
		PSK:     base64.StdEncoding.EncodeToString(pskBytes),
	}
	qrURL, shutdown, err := enrollqr.Serve(payload)
	if err != nil {
		return fmt.Errorf("start enroll QR server: %w", err)
	}
	fmt.Fprintf(out, "      QR URL: %s\n", qrURL)
	if opts.OpenBrowser != nil {
		_ = opts.OpenBrowser(qrURL)
	}
	fmt.Fprintln(out, "      Waiting for phone to check in via WebSocket (timeout 5 min)…")

	enrolled := waitForEnrollment(ctx, nickname)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(shutdownCtx)

	if !enrolled {
		return fmt.Errorf(
			"Phone %q did not check in within 5 minutes. Confirm: (1) Premex ADB-gate app installed on phone, "+
				"(2) Tailscale Android app installed and on the same tailnet, "+
				"(3) toggle ON in the Premex app, (4) QR was scanned.",
			nickname,
		)
	}
	fmt.Fprintf(out, "      Phone %q enrolled and online.\n", nickname)
	return nil
}

// ---- helpers ----------------------------------------------------------------

type daemonConfig struct {
	pskHex        string
	wsPort        int
	tailscaleHost string
}

func generateConfig(ctx context.Context, magicDNS func(ctx context.Context) string) (*daemonConfig, error) {
	if err := os.MkdirAll(paths.ConfigDir(), 0o700); err != nil {
		return nil, err
	}
	db, err := statestore.Open(paths.DBPath())
	if err != nil {
		return nil, fmt.Errorf("open state store: %w", err)
	}
	defer db.Close()

	existing, err := db.ServerConfig()
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &daemonConfig{
			pskHex:        hex.EncodeToString(existing.PSK),
			wsPort:        existing.WSPort,
			tailscaleHost: existing.TailscaleHost,
		}, nil
	}

	// Generate new PSK.
	psk := make([]byte, 32)
	if _, err := rand.Read(psk); err != nil {
		return nil, fmt.Errorf("generate psk: %w", err)
	}

	// Pick a free port.
	wsPort, err := pickFreePort()
	if err != nil {
		return nil, fmt.Errorf("pick free port: %w", err)
	}

	// Get Tailscale MagicDNS name from running tailscale.
	host := magicDNS(ctx)
	if host == "" {
		return nil, fmt.Errorf("tailscale MagicDNS name not available — is `tailscale up` complete?")
	}

	if err := db.SetServerConfig(&statestore.ServerConfig{
		PSK:           psk,
		WSPort:        wsPort,
		TailscaleHost: host,
	}); err != nil {
		return nil, fmt.Errorf("save server config: %w", err)
	}

	return &daemonConfig{
		pskHex:        hex.EncodeToString(psk),
		wsPort:        wsPort,
		tailscaleHost: host,
	}, nil
}

func pickFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

func requireAttachedPhone(ctx context.Context, devices func(context.Context) ([]adbDevice, error)) (adbDevice, error) {
	devs, err := devices(ctx)
	if err != nil {
		return adbDevice{}, fmt.Errorf("adb devices: %w", err)
	}
	var online []adbDevice
	for _, d := range devs {
		if d.State == "device" {
			online = append(online, d)
		}
	}
	switch len(online) {
	case 0:
		return adbDevice{}, fmt.Errorf(
			"no phone attached.\nConnect via USB (Developer options → USB debugging) or run /adb-connect:pair first, then re-run /adb-connect:remote-setup.",
		)
	case 1:
		return online[0], nil
	default:
		serials := make([]string, len(online))
		for i, d := range online {
			serials[i] = d.Serial
		}
		return adbDevice{}, fmt.Errorf(
			"multiple devices attached (%s). Leave only one connected for enrollment.",
			strings.Join(serials, ", "),
		)
	}
}

func promptAuthKey(opts RemoteSetupOpts, r *bufio.Reader) (string, error) {
	if !opts.NonInteractive {
		fmt.Fprintln(opts.Output, "\nPaste a reusable Tailscale auth key.")
		fmt.Fprintf(opts.Output, "Get one at https://login.tailscale.com/admin/settings/keys (select \"Reusable\").\n\n")
		fmt.Fprint(opts.Output, "Auth key: ")
	}
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read auth key: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func promptNickname(opts RemoteSetupOpts, r *bufio.Reader) (string, error) {
	if !opts.NonInteractive {
		fmt.Fprint(opts.Output, "Pick a nickname for this phone: ")
	}
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read nickname: %w", err)
	}
	nick := strings.TrimSpace(line)
	if !nicknameRE.MatchString(nick) {
		return "", fmt.Errorf("nickname must be lowercase letters, digits, hyphens (2–32 chars), got %q", nick)
	}
	return nick, nil
}

func resolveAPK(ctx context.Context, opts RemoteSetupOpts) (string, error) {
	useDownload := !opts.FromSource && opts.Version != "" && opts.Version != "dev"
	if useDownload {
		fmt.Fprintln(opts.Output, "     downloading Premex ADB-gate app…")
		dest := os.TempDir() + "/adb-gate-download.apk"
		if err := opts.apkDownloadFn(opts.Version, dest); err != nil {
			return "", fmt.Errorf("download APK: %w", err)
		}
		return dest, nil
	}

	// Source build path.
	androidHome := os.Getenv("ANDROID_HOME")
	if androidHome == "" {
		return "", fmt.Errorf("ANDROID_HOME not set; cannot build from source (use a release version instead)")
	}
	androidAppDir := findAndroidAppDir()
	fmt.Fprintln(opts.Output, "     building Premex ADB-gate app (this may take a minute)…")
	if err := opts.gradleAssembleFn(ctx, androidAppDir); err != nil {
		return "", fmt.Errorf("gradle assemble: %w", err)
	}
	apkPath := androidAppDir + "/app/build/outputs/apk/release/app-release.apk"
	if _, err := os.Stat(apkPath); err != nil {
		return "", fmt.Errorf("build succeeded but APK not found at %s", apkPath)
	}
	return apkPath, nil
}

func findAndroidAppDir() string {
	// Walk up from the executable location to find android-app/.
	exe, _ := os.Executable()
	dir := exe
	for i := 0; i < 5; i++ {
		dir = parentDir(dir)
		candidate := dir + "/android-app"
		if _, err := os.Stat(candidate + "/gradlew"); err == nil {
			return candidate
		}
	}
	// Fallback: relative to cwd.
	cwd, _ := os.Getwd()
	return cwd + "/android-app"
}

func parentDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "/"
}

// waitForEnrollment polls the IPC socket until nickname appears as online or timeout.
func waitForEnrollment(ctx context.Context, nickname string) bool {
	const pollInterval = 2 * time.Second
	const timeout = 5 * time.Minute
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		if checkPhoneOnline(nickname) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(pollInterval):
		}
	}
	return false
}

func checkPhoneOnline(nickname string) bool {
	conn, err := net.DialTimeout("unix", paths.IPCSocketPath(), 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := `{"op":"status"}` + "\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return false
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		return false
	}
	// Quick scan for the nickname+online pair without a full JSON unmarshal.
	s := sc.Text()
	return strings.Contains(s, `"`+nickname+`"`) && strings.Contains(s, `"online":true`)
}

// ---- production implementations of seams ------------------------------------

func defaultEnsureTSInstalled(ctx context.Context, platform string) error {
	if _, err := exec.LookPath("tailscale"); err == nil {
		return nil
	}
	switch platform {
	case "darwin":
		if _, err := exec.LookPath("brew"); err != nil {
			return fmt.Errorf("Homebrew not found. Install from https://brew.sh/ and re-run")
		}
		if err := runCmd(ctx, "brew", "install", "tailscale"); err != nil {
			return err
		}
		return runCmd(ctx, "sudo", "brew", "services", "start", "tailscale")
	case "linux":
		return runCmd(ctx, "sh", "-c", "curl -fsSL https://tailscale.com/install.sh | sh")
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
}

func defaultEnsureTSUp(ctx context.Context, key string) error {
	// Already up? The status has to report Running AND a MagicDNS name.
	if s := tailscale.GetStatus(ctx); s != nil && s.BackendState == "Running" && s.Self.DNSName != "" {
		return nil
	}
	if key == "" {
		return fmt.Errorf("no tailscale auth key provided")
	}
	if !strings.HasPrefix(key, "tskey-") {
		return fmt.Errorf("invalid auth key format (expected tskey-…)")
	}
	return runCmd(ctx, "sudo", "tailscale", "up", "--auth-key="+key)
}

func defaultADBDevices(ctx context.Context) ([]adbDevice, error) {
	cmd := exec.CommandContext(ctx, "adb", "devices")
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, nil
	}
	var devs []adbDevice
	for i, line := range strings.Split(out.String(), "\n") {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 {
			devs = append(devs, adbDevice{Serial: f[0], State: f[1]})
		}
	}
	return devs, nil
}

func defaultADBInstall(ctx context.Context, apkPath string) error {
	cmd := exec.CommandContext(ctx, "adb", "install", "-r", apkPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultADBGrant(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "adb", "shell", "pm", "grant", pkg, "android.permission.WRITE_SECURE_SETTINGS")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultGradleAssemble(ctx context.Context, dir string) error {
	gradlew := "./gradlew"
	if runtime.GOOS == "windows" {
		gradlew = "gradlew.bat"
	}
	cmd := exec.CommandContext(ctx, gradlew, ":app:assembleRelease")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
