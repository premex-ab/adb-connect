// Package paths resolves XDG-compliant filesystem locations for adb-connect.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "adb-connect"

func home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// ConfigDir returns the per-user directory for persistent daemon state.
// macOS: ~/Library/Application Support/adb-connect
// Linux: $XDG_CONFIG_HOME/adb-connect or ~/.config/adb-connect
func ConfigDir() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home(), "Library", "Application Support", appName)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, appName)
	}
	return filepath.Join(home(), ".config", appName)
}

// LogDir returns the per-user directory for daemon logs.
func LogDir() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home(), "Library", "Logs", appName)
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, appName)
	}
	return filepath.Join(home(), ".local", "state", appName)
}

func DBPath() string        { return filepath.Join(ConfigDir(), "devices.db") }
func IPCSocketPath() string { return filepath.Join(ConfigDir(), "daemon.sock") }
func LogPath() string       { return filepath.Join(LogDir(), "server.log") }
