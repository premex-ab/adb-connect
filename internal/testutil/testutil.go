// Package testutil provides tmp-dir and env-override helpers for tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempHome creates a temporary HOME for a test and restores the previous HOME on cleanup.
// On macOS os.TempDir() can produce sockaddr paths that exceed 104 bytes when combined with
// subdirectories, so we place our temp under /tmp which is shorter.
func TempHome(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if _, err := os.Stat("/tmp"); err == nil {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "adbc-test-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	prevHome := os.Getenv("HOME")
	prevXDG := os.Getenv("XDG_CONFIG_HOME")
	prevState := os.Getenv("XDG_STATE_HOME")
	os.Setenv("HOME", dir)
	os.Unsetenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_STATE_HOME")
	t.Cleanup(func() {
		os.Setenv("HOME", prevHome)
		if prevXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", prevXDG)
		}
		if prevState != "" {
			os.Setenv("XDG_STATE_HOME", prevState)
		}
		os.RemoveAll(dir)
	})
	return dir
}

// FakeBinary writes an executable shell script under a scratch directory that
// is prepended to $PATH, then restores the original $PATH on cleanup.
// Use to mock external CLIs (adb, tailscale, brew, …) in tests.
func FakeBinary(t *testing.T, name, script string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "fake-bin-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	prev := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+prev)
	t.Cleanup(func() {
		os.Setenv("PATH", prev)
		os.RemoveAll(dir)
	})
}
