package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/premex-ab/adb-connect/internal/adb"
	"github.com/premex-ab/adb-connect/internal/apk"
	"github.com/premex-ab/adb-connect/internal/version"
)

const appPackage = "se.premex.adbgate"

func newInstallAppCmd() *cobra.Command {
	var serial string
	cmd := &cobra.Command{
		Use:   "install-app",
		Short: "Sideload the signed Premex ADB-gate companion app onto an attached phone",
		Long: `Downloads the signed Premex ADB-gate APK matching this CLI's version from GitHub
Releases, verifies its SHA-256 against the published checksum file, then runs
'adb install -r' followed by 'adb shell pm grant se.premex.adbgate WRITE_SECURE_SETTINGS'.

The phone must already be attached via 'adb devices' — USB or already wifi-paired.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ver := version.Version
			if ver == "dev" || ver == "" {
				return fmt.Errorf("this is a dev build; no published APK to download — install from a tagged release")
			}
			ver = strings.TrimPrefix(ver, "v")
			return runInstallApp(cmd, ver, serial)
		},
	}
	cmd.Flags().StringVarP(&serial, "serial", "s", "", "target a specific adb device by serial (default: auto-pick if one device is attached)")
	return cmd
}

func runInstallApp(cmd *cobra.Command, ver, serial string) error {
	ctx := cmd.Context()

	// Ensure exactly one device (or the specified serial) is attached.
	devs, err := adb.Devices(ctx)
	if err != nil {
		return fmt.Errorf("adb devices: %w", err)
	}
	var online []adb.Device
	for _, d := range devs {
		if d.State == "device" {
			online = append(online, d)
		}
	}
	if serial != "" {
		found := false
		for _, d := range online {
			if d.Serial == serial {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("device %q not attached in state 'device'", serial)
		}
	} else {
		switch len(online) {
		case 0:
			return fmt.Errorf("no phone attached — connect via USB (Developer options → USB debugging) or 'adb-connect pair' first")
		case 1:
			serial = online[0].Serial
		default:
			names := make([]string, len(online))
			for i, d := range online {
				names[i] = d.Serial
			}
			return fmt.Errorf("multiple devices attached (%s) — pass --serial <S>", strings.Join(names, ", "))
		}
	}

	// Download and verify APK into a temp dir.
	tmpDir, err := os.MkdirTemp("", "adb-connect-apk-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	apkFile := fmt.Sprintf("adb-gate-%s.apk", ver)
	apkPath := filepath.Join(tmpDir, apkFile)

	fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s (v%s)…\n", apkFile, ver)
	if err := apk.Download(ver, apkPath); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "APK SHA-256 verified.")

	// adb install -r.
	fmt.Fprintln(cmd.OutOrStdout(), "Installing on device…")
	r, _ := adb.Install(ctx, apkPath)
	if !r.OK {
		return fmt.Errorf("adb install failed: %s", strings.TrimSpace(r.Stderr))
	}

	// Grant WRITE_SECURE_SETTINGS.
	fmt.Fprintln(cmd.OutOrStdout(), "Granting WRITE_SECURE_SETTINGS…")
	r, _ = adb.GrantWriteSecureSettings(ctx, appPackage)
	if !r.OK {
		return fmt.Errorf("pm grant failed: %s", strings.TrimSpace(r.Stderr))
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Done. Open Premex ADB-gate on the phone and flip the toggle ON.")
	return nil
}
