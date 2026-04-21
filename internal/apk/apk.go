// Package apk downloads the signed Premex ADB-gate APK from the GitHub release
// matching the CLI version and verifies its SHA-256 against a value fetched
// from the same release's manifest.json.
//
// SetReleaseBase is a test-only hook that redirects downloads to a fake server.
// Production code should never call it.
package apk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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

// Download writes the signed APK for version v to destPath and verifies the SHA against the release manifest.
func Download(version, destPath string) error {
	if version == "" || version == "dev" {
		return errors.New("apk: no release version — cannot download pre-built APK for dev builds")
	}
	manifestURL := fmt.Sprintf("%s/v%s/manifest.json", releaseBase, version)
	mresp, err := httpGet(manifestURL)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}
	defer mresp.Body.Close()
	var mf struct {
		APK struct {
			Filename string `json:"filename"`
			SHA256   string `json:"sha256"`
		} `json:"apk"`
	}
	if err := json.NewDecoder(mresp.Body).Decode(&mf); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	apkURL := fmt.Sprintf("%s/v%s/%s", releaseBase, version, mf.APK.Filename)
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
	if got != mf.APK.SHA256 {
		_ = os.Remove(destPath)
		return fmt.Errorf("apk: sha256 mismatch (got %s, want %s)", got, mf.APK.SHA256)
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
