package apk_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/premex-ab/adb-connect/internal/apk"
)

// fakeRelease serves a manifest and APK that match. Returns a base URL to rewrite apk.releaseBase.
func fakeRelease(t *testing.T, apkBytes []byte) string {
	t.Helper()
	sum := sha256.Sum256(apkBytes)
	sumHex := hex.EncodeToString(sum[:])
	manifest := fmt.Sprintf(`{"apk":{"filename":"adb-gate-0.1.0.apk","sha256":"%s"}}`, sumHex)
	mux := http.NewServeMux()
	mux.HandleFunc("/v0.1.0/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/v0.1.0/adb-gate-0.1.0.apk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(apkBytes)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestDownload_Success(t *testing.T) {
	apkBytes := []byte("fake APK content " + strings.Repeat("x", 1000))
	base := fakeRelease(t, apkBytes)
	// patch the package-level base at runtime using apk.SetReleaseBase (to be added in apk.go)
	oldBase := apk.SetReleaseBase(base)
	defer apk.SetReleaseBase(oldBase)

	dest := filepath.Join(t.TempDir(), "out.apk")
	if err := apk.Download("0.1.0", dest); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(apkBytes) {
		t.Fatalf("content mismatch")
	}
}

func TestDownload_SHAMismatch(t *testing.T) {
	base := func() string {
		mux := http.NewServeMux()
		// manifest says one SHA, apk body has different SHA
		mux.HandleFunc("/v0.1.0/manifest.json", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"apk":{"filename":"x.apk","sha256":"deadbeef"}}`))
		})
		mux.HandleFunc("/v0.1.0/x.apk", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("some bytes"))
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		return srv.URL
	}()
	oldBase := apk.SetReleaseBase(base)
	defer apk.SetReleaseBase(oldBase)
	dest := filepath.Join(t.TempDir(), "out.apk")
	if err := apk.Download("0.1.0", dest); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want sha256 mismatch err, got %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("apk file should be removed on SHA mismatch, got err=%v", err)
	}
}

func TestDownload_ManifestMissing(t *testing.T) {
	mux := http.NewServeMux() // no handlers -> 404
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldBase := apk.SetReleaseBase(srv.URL)
	defer apk.SetReleaseBase(oldBase)
	dest := filepath.Join(t.TempDir(), "out.apk")
	if err := apk.Download("0.1.0", dest); err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestDownload_DevVersionRejected(t *testing.T) {
	if err := apk.Download("dev", ""); err == nil {
		t.Fatal("dev should be rejected")
	}
}
