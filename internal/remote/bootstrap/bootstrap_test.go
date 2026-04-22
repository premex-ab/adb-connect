package bootstrap

// Tests live in package bootstrap (not bootstrap_test) so they can populate
// the unexported seam fields on RemoteSetupOpts without a separate export shim.

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/remote/service"
	"github.com/premex-ab/adb-connect/internal/tailscale"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

// ---- seam helpers -----------------------------------------------------------

func noopTSInstalled(_ context.Context, _ string) error { return nil }
func noopTSUp(_ context.Context, _ string) error        { return nil }

// fakeTSStatusNotRunning returns nil so RemoteSetup takes the auth-key prompt path.
// Most tests use this because their Input reader queues up an auth-key line first.
func fakeTSStatusNotRunning(_ context.Context) *tailscale.Status { return nil }

// fakeTSStatusRunning simulates tailscale already up; RemoteSetup skips the auth-key prompt.
func fakeTSStatusRunning(_ context.Context) *tailscale.Status {
	s := &tailscale.Status{BackendState: "Running"}
	s.Self.DNSName = "test-host.ts.net."
	return s
}

// fakeTSMagicDNS returns a deterministic MagicDNS host for generateConfig.
func fakeTSMagicDNS(_ context.Context) string        { return "test-host.ts.net" }
func noopInstall(_ context.Context, _ string) error  { return nil }
func noopGrant(_ context.Context, _ string) error    { return nil }
func noopGradle(_ context.Context, _ string) error   { return nil }
func noopAPKDownload(_, _ string) error              { return nil }
func noopServiceInstall(_ service.InstallOpts) error { return nil }

func onePhone(_ context.Context) ([]adbDevice, error) {
	return []adbDevice{{Serial: "emulator-5554", State: "device"}}, nil
}

// fakeTwoPhones returns two attached phones.
func fakeTwoPhones(_ context.Context) ([]adbDevice, error) {
	return []adbDevice{
		{Serial: "emulator-5554", State: "device"},
		{Serial: "emulator-5556", State: "device"},
	}, nil
}

// noPhones returns an empty list.
func noPhones(_ context.Context) ([]adbDevice, error) {
	return nil, nil
}

// fakeIPCServer starts a Unix-domain socket that responds to {"op":"status"}
// with the given nickname shown as online=true. Returns a cleanup func.
func fakeIPCServer(t *testing.T, nickname string) func() {
	t.Helper()
	sockPath := paths.IPCSocketPath()
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)
	_ = os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("fakeIPCServer listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, _ := c.Read(buf)
				var req map[string]any
				_ = json.Unmarshal(buf[:n], &req)
				resp := map[string]any{
					"ok": true,
					"phones": []map[string]any{
						{"nickname": nickname, "online": true},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = c.Write(append(b, '\n'))
			}(c)
		}
	}()
	return func() {
		_ = ln.Close()
		_ = os.Remove(sockPath)
	}
}

// ---- tests ------------------------------------------------------------------

// TestRemoteSetup_Success_APKDownload exercises the happy path with APK download.
func TestRemoteSetup_Success_APKDownload(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)

	// Pre-seed the server config so generateConfig skips tailscale status.
	db, err := statestore.Open(paths.DBPath())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.SetServerConfig(&statestore.ServerConfig{
		PSK:           []byte("01234567890123456789012345678901"),
		WSPort:        19876,
		TailscaleHost: "myhost.tailnet.ts.net",
	}); err != nil {
		_ = db.Close()
		t.Fatalf("set server config: %v", err)
	}
	_ = db.Close()

	stopIPC := fakeIPCServer(t, "pixel")
	defer stopIPC()

	var serviceInstallCalled bool
	var apkDownloadCalled bool
	var urlPrinted string
	out := &strings.Builder{}

	opts := RemoteSetupOpts{
		Version:        "0.1.0",
		FromSource:     false,
		NonInteractive: true,
		// auth key read from Input, then nickname read from Input
		Input:  strings.NewReader("tskey-auth-fake\npixel\n"),
		Output: out,
		OpenBrowser: func(url string) error {
			urlPrinted = url
			return nil
		},
		ensureTSInstalled: noopTSInstalled,
		tsStatusFn:        fakeTSStatusNotRunning,
		tsMagicDNSFn:      fakeTSMagicDNS,
		ensureTSUp:        noopTSUp,
		adbDevicesFn:      onePhone,
		adbInstallFn:      noopInstall,
		adbGrantFn:        noopGrant,
		apkDownloadFn: func(version, dest string) error {
			apkDownloadCalled = true
			// Create a placeholder file so the download path "exists".
			_ = os.WriteFile(dest, []byte("fake apk"), 0o644)
			return nil
		},
		serviceInstallFn: func(o service.InstallOpts) error {
			serviceInstallCalled = true
			return nil
		},
	}

	if err := RemoteSetup(context.Background(), opts); err != nil {
		t.Fatalf("RemoteSetup: %v", err)
	}

	if !serviceInstallCalled {
		t.Error("expected serviceInstall to be called")
	}
	if !apkDownloadCalled {
		t.Error("expected apkDownload to be called")
	}
	if !strings.Contains(out.String(), "QR URL") {
		t.Errorf("expected QR URL in output, got: %s", out.String())
	}
	if !strings.Contains(urlPrinted, "http://") {
		t.Errorf("expected OpenBrowser called with http URL, got: %q", urlPrinted)
	}

	// Verify state store was seeded with the phone.
	db2, err := statestore.Open(paths.DBPath())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db2.Close()
	ph, err := db2.GetPhone("pixel")
	if err != nil {
		t.Fatalf("get phone: %v", err)
	}
	if ph == nil {
		t.Fatal("expected phone 'pixel' in state store")
	}
}

// TestRemoteSetup_Success_SourceBuild exercises source build when FromSource=true and ANDROID_HOME set.
func TestRemoteSetup_Success_SourceBuild(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)

	// Pre-seed server config.
	db, _ := statestore.Open(paths.DBPath())
	_ = db.SetServerConfig(&statestore.ServerConfig{
		PSK:           []byte("01234567890123456789012345678901"),
		WSPort:        19877,
		TailscaleHost: "myhost.tailnet.ts.net",
	})
	_ = db.Close()

	stopIPC := fakeIPCServer(t, "mypixel")
	defer stopIPC()

	// Set ANDROID_HOME so source build path is taken.
	prevAH := os.Getenv("ANDROID_HOME")
	os.Setenv("ANDROID_HOME", "/fake/android")
	defer os.Setenv("ANDROID_HOME", prevAH)

	// Create a fake APK at the expected output path so resolveAPK stat check passes.
	androidAppDir := findAndroidAppDir()
	apkOutDir := androidAppDir + "/app/build/outputs/apk/release"
	_ = os.MkdirAll(apkOutDir, 0o755)
	_ = os.WriteFile(apkOutDir+"/app-release.apk", []byte("fake"), 0o644)
	defer os.RemoveAll(androidAppDir + "/app")

	var gradleCalled bool
	var apkDownloadCalled bool

	opts := RemoteSetupOpts{
		Version:           "dev",
		FromSource:        true,
		NonInteractive:    true,
		Input:             strings.NewReader("tskey-auth-fake\nmypixel\n"),
		Output:            &strings.Builder{},
		ensureTSInstalled: noopTSInstalled,
		tsStatusFn:        fakeTSStatusNotRunning,
		tsMagicDNSFn:      fakeTSMagicDNS,
		ensureTSUp:        noopTSUp,
		adbDevicesFn:      onePhone,
		adbInstallFn:      noopInstall,
		adbGrantFn:        noopGrant,
		gradleAssembleFn: func(_ context.Context, dir string) error {
			gradleCalled = true
			return nil
		},
		apkDownloadFn: func(version, dest string) error {
			apkDownloadCalled = true
			return nil
		},
		serviceInstallFn: noopServiceInstall,
	}

	if err := RemoteSetup(context.Background(), opts); err != nil {
		t.Fatalf("RemoteSetup: %v", err)
	}
	if !gradleCalled {
		t.Error("expected gradle to be called for source build")
	}
	if apkDownloadCalled {
		t.Error("apkDownload should NOT be called in source build mode")
	}
}

// TestRemoteSetup_NoPhone returns an error when no devices are attached.
func TestRemoteSetup_NoPhone(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)

	db, _ := statestore.Open(paths.DBPath())
	_ = db.SetServerConfig(&statestore.ServerConfig{
		PSK:           []byte("01234567890123456789012345678901"),
		WSPort:        19878,
		TailscaleHost: "myhost.tailnet.ts.net",
	})
	_ = db.Close()

	opts := RemoteSetupOpts{
		Version:           "0.1.0",
		NonInteractive:    true,
		Input:             strings.NewReader("tskey-auth-fake\n"),
		Output:            &strings.Builder{},
		ensureTSInstalled: noopTSInstalled,
		tsStatusFn:        fakeTSStatusNotRunning,
		tsMagicDNSFn:      fakeTSMagicDNS,
		ensureTSUp:        noopTSUp,
		adbDevicesFn:      noPhones,
		adbInstallFn:      noopInstall,
		adbGrantFn:        noopGrant,
		apkDownloadFn:     noopAPKDownload,
		serviceInstallFn:  noopServiceInstall,
	}

	err := RemoteSetup(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for no phone attached")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "no phone attached") {
		t.Errorf("error should mention 'no phone attached', got: %v", err)
	}
}

// TestRemoteSetup_MultiplePhones returns an error listing serials when multiple devices attached.
func TestRemoteSetup_MultiplePhones(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)

	db, _ := statestore.Open(paths.DBPath())
	_ = db.SetServerConfig(&statestore.ServerConfig{
		PSK:           []byte("01234567890123456789012345678901"),
		WSPort:        19879,
		TailscaleHost: "myhost.tailnet.ts.net",
	})
	_ = db.Close()

	opts := RemoteSetupOpts{
		Version:           "0.1.0",
		NonInteractive:    true,
		Input:             strings.NewReader("tskey-auth-fake\n"),
		Output:            &strings.Builder{},
		ensureTSInstalled: noopTSInstalled,
		tsStatusFn:        fakeTSStatusNotRunning,
		tsMagicDNSFn:      fakeTSMagicDNS,
		ensureTSUp:        noopTSUp,
		adbDevicesFn:      fakeTwoPhones,
		adbInstallFn:      noopInstall,
		adbGrantFn:        noopGrant,
		apkDownloadFn:     noopAPKDownload,
		serviceInstallFn:  noopServiceInstall,
	}

	err := RemoteSetup(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for multiple phones")
	}
	if !strings.Contains(err.Error(), "emulator-5554") || !strings.Contains(err.Error(), "emulator-5556") {
		t.Errorf("error should list serials, got: %v", err)
	}
}

// TestRemoteSetup_InvalidNickname returns a validation error for a non-matching nickname.
func TestRemoteSetup_InvalidNickname(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)

	db, _ := statestore.Open(paths.DBPath())
	_ = db.SetServerConfig(&statestore.ServerConfig{
		PSK:           []byte("01234567890123456789012345678901"),
		WSPort:        19880,
		TailscaleHost: "myhost.tailnet.ts.net",
	})
	_ = db.Close()

	opts := RemoteSetupOpts{
		Version:        "0.1.0",
		NonInteractive: true,
		// auth key first, then the invalid nickname
		Input:             strings.NewReader("tskey-auth-fake\nUPPER.invalid\n"),
		Output:            &strings.Builder{},
		ensureTSInstalled: noopTSInstalled,
		tsStatusFn:        fakeTSStatusNotRunning,
		tsMagicDNSFn:      fakeTSMagicDNS,
		ensureTSUp:        noopTSUp,
		adbDevicesFn:      onePhone,
		adbInstallFn:      noopInstall,
		adbGrantFn:        noopGrant,
		apkDownloadFn:     noopAPKDownload,
		serviceInstallFn:  noopServiceInstall,
	}

	err := RemoteSetup(context.Background(), opts)
	if err == nil {
		t.Fatal("expected validation error for invalid nickname")
	}
	if !strings.Contains(err.Error(), "nickname must be") {
		t.Errorf("expected nickname validation error, got: %v", err)
	}
}
