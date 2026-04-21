package paths_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestConfigDir_XDGOrAppSupport(t *testing.T) {
	home := testutil.TempHome(t)
	got := paths.ConfigDir()
	var want string
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Application Support", "adb-connect")
	} else {
		want = filepath.Join(home, ".config", "adb-connect")
	}
	if got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}
}

func TestHelpersAreUnderExpectedDirs(t *testing.T) {
	testutil.TempHome(t)
	if !strings.HasPrefix(paths.DBPath(), paths.ConfigDir()) {
		t.Errorf("DBPath not under ConfigDir: %s", paths.DBPath())
	}
	if !strings.HasPrefix(paths.IPCSocketPath(), paths.ConfigDir()) {
		t.Errorf("IPCSocketPath not under ConfigDir: %s", paths.IPCSocketPath())
	}
	if !strings.Contains(paths.LogPath(), "adb-connect") {
		t.Errorf("LogPath does not contain adb-connect: %s", paths.LogPath())
	}
}
