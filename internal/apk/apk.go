// Package apk downloads the signed Premex ADB-gate APK from the GitHub release
// matching the CLI version and verifies its SHA-256 against the published
// <apk-filename>.sha256 file uploaded alongside it.
//
// SetReleaseBase is a test-only hook that redirects downloads to a fake server.
// Production code should never call it.
package apk

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var releaseBase = "https://github.com/premex-ab/adb-connect/releases/download"

// SetReleaseBase replaces the release base URL and returns the previous value.
// Test-only hook; package semantics are unaffected in production.
func SetReleaseBase(newBase string) string {
	prev := releaseBase
	releaseBase = newBase
	return prev
}

// Download writes the signed APK for version v to destPath and verifies the SHA
// against the <apk-filename>.sha256 file published alongside the release asset.
func Download(version, destPath string) error {
	if version == "" || version == "dev" {
		return errors.New("apk: no release version — cannot download pre-built APK for dev builds")
	}
	apkFile := fmt.Sprintf("adb-gate-%s.apk", version)
	baseURL := fmt.Sprintf("%s/v%s", releaseBase, version)

	// Download the checksum file first (small, fast).
	checksumURL := baseURL + "/" + apkFile + ".sha256"
	sumResp, err := httpGet(checksumURL)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}
	defer sumResp.Body.Close()
	sumBytes, err := io.ReadAll(sumResp.Body)
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	fields := strings.Fields(string(sumBytes))
	if len(fields) < 1 {
		return fmt.Errorf("apk: empty checksum file at %s", checksumURL)
	}
	wantSHA := fields[0]

	// Download the APK.
	apkURL := baseURL + "/" + apkFile
	aresp, err := httpGet(apkURL)
	if err != nil {
		return fmt.Errorf("fetch apk: %w", err)
	}
	defer aresp.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), aresp.Body); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != wantSHA {
		_ = os.Remove(destPath)
		return fmt.Errorf("apk: sha256 mismatch (got %s, want %s)", got, wantSHA)
	}
	return nil
}

func httpGet(url string) (*http.Response, error) {
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}
