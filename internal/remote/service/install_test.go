package service_test

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/service"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestInstall_MacPlistDryRun(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only")
	}
	home := testutil.TempHome(t)
	err := service.Install(service.InstallOpts{BinaryPath: "/usr/local/bin/adb-connect", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	p := home + "/Library/LaunchAgents/se.premex.adbgate-server.plist"
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"se.premex.adbgate-server", "/usr/local/bin/adb-connect", "<string>daemon</string>", "RunAtLoad"} {
		if !strings.Contains(s, want) {
			t.Errorf("plist missing %q", want)
		}
	}
}

func TestInstall_LinuxUnitDryRun(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	home := testutil.TempHome(t)
	err := service.Install(service.InstallOpts{BinaryPath: "/usr/local/bin/adb-connect", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	p := home + "/.config/systemd/user/adb-connect-server.service"
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"ExecStart=/usr/local/bin/adb-connect daemon", "Restart=always", "WantedBy=default.target"} {
		if !strings.Contains(s, want) {
			t.Errorf("unit missing %q", want)
		}
	}
}

func TestInstall_UnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		t.Skip("skip on supported platforms")
	}
	if err := service.Install(service.InstallOpts{BinaryPath: "x", DryRun: true}); err == nil {
		t.Fatal("expected error on unsupported platform")
	}
}
