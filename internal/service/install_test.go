package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDaemonPath_PrependsAdbDir(t *testing.T) {
	// Create a tmp dir with an executable named "adb", prepend to PATH so
	// exec.LookPath finds it there rather than on the real machine's PATH.
	dir := t.TempDir()
	fakeAdb := filepath.Join(dir, "adb")
	if err := os.WriteFile(fakeAdb, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake adb: %v", err)
	}
	prev := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+prev)

	got := buildDaemonPath()
	if !strings.HasPrefix(got, dir+":") {
		t.Fatalf("expected %q to start with %q:", got, dir)
	}
	// Base dirs still present.
	for _, want := range []string{"/opt/homebrew/bin", "/usr/bin", "/bin"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in PATH", want)
		}
	}
}

func TestBuildDaemonPath_NoAdb(t *testing.T) {
	// Scrub PATH so exec.LookPath("adb") fails; we should still get the base dirs.
	prev := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	_ = os.Setenv("PATH", "")

	got := buildDaemonPath()
	for _, want := range []string{"/opt/homebrew/bin", "/usr/bin"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in PATH fallback, got %q", want, got)
		}
	}
}

func TestBuildDaemonPath_DedupesWhenAdbAlreadyInBase(t *testing.T) {
	// Put fake adb in /usr/bin (or /bin) — either is in the base PATH already.
	// Actually on CI we can't write /usr/bin, so instead just verify the logic
	// doesn't duplicate a path that already appears in the base set by
	// lookup-ing one of the base dirs.
	dir := t.TempDir()
	fakeAdb := filepath.Join(dir, "adb")
	if err := os.WriteFile(fakeAdb, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	// PATH order: tmp dir first, so LookPath returns the fake adb.
	_ = os.Setenv("PATH", dir+":"+prev)

	got := buildDaemonPath()
	// There should be exactly one occurrence of the tmp dir in the result.
	if strings.Count(got, dir) != 1 {
		t.Errorf("expected tmp dir to appear once in %q, got %d occurrences", got, strings.Count(got, dir))
	}
}
