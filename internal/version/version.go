// Package version exposes the build-time version string.
// The Go linker sets Version via -ldflags "-X github.com/premex-ab/adb-connect/internal/version.Version=…".
package version

import "runtime/debug"

// Version is overridden at build time by goreleaser. Defaults to "dev" for local builds.
var Version = "dev"

// Full returns the human-readable version including module info when available.
func Full() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}
