// Package service installs and removes the adb-connect watcher as a
// platform-native background service.
//
// macOS:   launchd user agent (~~/Library/LaunchAgents/se.premex.adb-connect-watch.plist)
// Linux:   systemd-user unit   (~/.config/systemd/user/adb-connect-watch.service)
//
// The launchd plist includes an explicit PATH that covers Homebrew prefixes so
// that `adb` is found even when launchd's own minimal PATH is in effect.
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const launchdLabel = "se.premex.adb-connect-watch"

// launchdPlistTemplate is the macOS launchd user-agent plist. Placeholders:
//   - {{BINARY}} — absolute path to the adb-connect binary
//   - {{HOME}}   — user's home directory
const launchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>se.premex.adb-connect-watch</string>
  <key>ProgramArguments</key>
  <array><string>{{BINARY}}</string><string>watch</string></array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>{{HOME}}/Library/Logs/adb-connect/watch.log</string>
  <key>StandardErrorPath</key><string>{{HOME}}/Library/Logs/adb-connect/watch.log</string>
</dict>
</plist>
`

// systemdUnitTemplate is the Linux systemd-user unit. Placeholder:
//   - {{BINARY}} — absolute path to the adb-connect binary
const systemdUnitTemplate = `[Unit]
Description=adb-connect mDNS auto-connect watcher
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{BINARY}} watch
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`

// InstallOpts controls the installation.
type InstallOpts struct {
	// BinaryPath is the absolute path to the adb-connect executable.
	// Typically obtained via os.Executable() in the caller.
	BinaryPath string
}

// Install writes the platform-appropriate service file and activates it.
// On macOS it calls `launchctl load`; on Linux it calls
// `systemctl --user daemon-reload && systemctl --user enable --now`.
func Install(opts InstallOpts) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("service install: home dir: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(opts.BinaryPath, home)
	case "linux":
		return installSystemd(opts.BinaryPath, home)
	default:
		return fmt.Errorf("service install: unsupported OS %q; run `%s watch` manually", runtime.GOOS, opts.BinaryPath)
	}
}

// Uninstall stops and removes the platform service. Errors are printed to
// stderr but not returned — partial removal is better than nothing.
func Uninstall() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "service uninstall: home dir:", err)
		return
	}
	switch runtime.GOOS {
	case "darwin":
		uninstallLaunchd(home)
	case "linux":
		uninstallSystemd(home)
	default:
		fmt.Fprintf(os.Stderr, "service uninstall: unsupported OS %q\n", runtime.GOOS)
	}
}

// ---- macOS (launchd) -------------------------------------------------------

func launchAgentsDir(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents")
}

func launchdPlistPath(home string) string {
	return filepath.Join(launchAgentsDir(home), launchdLabel+".plist")
}

func installLaunchd(binary, home string) error {
	// Ensure log dir exists.
	logDir := filepath.Join(home, "Library", "Logs", "adb-connect")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("launchd install: create log dir: %w", err)
	}

	if err := os.MkdirAll(launchAgentsDir(home), 0o755); err != nil {
		return fmt.Errorf("launchd install: create LaunchAgents dir: %w", err)
	}

	content := strings.ReplaceAll(launchdPlistTemplate, "{{BINARY}}", binary)
	content = strings.ReplaceAll(content, "{{HOME}}", home)

	plistPath := launchdPlistPath(home)
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("launchd install: write plist: %w", err)
	}

	// Unload first in case an old version is loaded.
	_ = runQuiet("launchctl", "unload", plistPath)

	if err := runQuiet("launchctl", "load", plistPath); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}
	return nil
}

func uninstallLaunchd(home string) {
	plistPath := launchdPlistPath(home)
	_ = runQuiet("launchctl", "unload", plistPath)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "launchd uninstall: remove plist:", err)
	}
}

// ---- Linux (systemd-user) --------------------------------------------------

func systemdUserDir(home string) string {
	return filepath.Join(home, ".config", "systemd", "user")
}

func systemdUnitPath(home string) string {
	return filepath.Join(systemdUserDir(home), "adb-connect-watch.service")
}

func installSystemd(binary, home string) error {
	if err := os.MkdirAll(systemdUserDir(home), 0o755); err != nil {
		return fmt.Errorf("systemd install: create unit dir: %w", err)
	}

	content := strings.ReplaceAll(systemdUnitTemplate, "{{BINARY}}", binary)

	unitPath := systemdUnitPath(home)
	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("systemd install: write unit: %w", err)
	}

	if err := runQuiet("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if err := runQuiet("systemctl", "--user", "enable", "--now", "adb-connect-watch.service"); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	return nil
}

func uninstallSystemd(home string) {
	_ = runQuiet("systemctl", "--user", "disable", "--now", "adb-connect-watch.service")
	unitPath := systemdUnitPath(home)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "systemd uninstall: remove unit:", err)
	}
	_ = runQuiet("systemctl", "--user", "daemon-reload")
}

// runQuiet runs a command, combining stderr into the returned error on failure.
func runQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
