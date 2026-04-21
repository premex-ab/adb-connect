// Package service installs adb-connect as a user-level service (launchd on macOS, systemd --user on Linux).
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const label = "se.premex.adbgate-server"

// InstallOpts controls service installation behaviour.
type InstallOpts struct {
	BinaryPath string // absolute path to adb-connect binary
	DryRun     bool   // skip the actual load/enable step
}

func macPlistPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", label+".plist")
}
func linuxUnitPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "adb-connect-server.service")
}
func macLogsDir() string { return filepath.Join(os.Getenv("HOME"), "Library", "Logs", "adb-connect") }

// Install writes the service definition file and, unless DryRun is set, loads/enables the service.
func Install(opts InstallOpts) error {
	switch runtime.GOOS {
	case "darwin":
		return installMac(opts)
	case "linux":
		return installLinux(opts)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func installMac(o InstallOpts) error {
	_ = os.MkdirAll(filepath.Dir(macPlistPath()), 0o755)
	_ = os.MkdirAll(macLogsDir(), 0o755)
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>daemon</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s/stdout.log</string>
  <key>StandardErrorPath</key><string>%s/stderr.log</string>
</dict>
</plist>
`, label, o.BinaryPath, macLogsDir(), macLogsDir())
	if err := os.WriteFile(macPlistPath(), []byte(content), 0o644); err != nil {
		return err
	}
	if o.DryRun {
		return nil
	}
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), label)).Run()
	return exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), macPlistPath()).Run()
}

func installLinux(o InstallOpts) error {
	_ = os.MkdirAll(filepath.Dir(linuxUnitPath()), 0o755)
	content := fmt.Sprintf(`[Unit]
Description=adb-connect remote daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s daemon
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`, o.BinaryPath)
	if err := os.WriteFile(linuxUnitPath(), []byte(content), 0o644); err != nil {
		return err
	}
	if o.DryRun {
		return nil
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return exec.Command("systemctl", "--user", "enable", "--now", "adb-connect-server.service").Run()
}

// Uninstall stops and removes the service definition for the current platform.
func Uninstall() {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), label)).Run()
		_ = os.Remove(macPlistPath())
	case "linux":
		_ = exec.Command("systemctl", "--user", "disable", "--now", "adb-connect-server.service").Run()
		_ = os.Remove(linuxUnitPath())
	}
}
