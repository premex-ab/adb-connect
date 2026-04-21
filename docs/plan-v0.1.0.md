# adb-connect v0.1.0 — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship premex-ab/adb-connect v0.1.0 — a Go CLI + migrated Kotlin Android app + signed-APK release pipeline, delivering the Tailscale-remote-ADB flow proven end-to-end on real hardware in the Claude-plugin precursor.

**Architecture:** Monorepo, single Go binary (cobra CLI, no CGO) that subsumes daemon and commands; Kotlin + Compose Android companion app migrated verbatim from `claude-marketplace/plugins/adb-connect/android-app/`; Goreleaser release pipeline publishes cross-compiled binaries to GitHub Releases + Homebrew tap; Android CI job signs + publishes the APK alongside.

**Tech stack:**
- Go 1.22+, `spf13/cobra`, `nhooyr.io/websocket`, `modernc.org/sqlite`, `grandcat/zeroconf`, `skip2/go-qrcode`, stdlib `log/slog`.
- Kotlin 1.9 + Jetpack Compose, Android min SDK 30, Google MLKit barcode-scanning, OkHttp WebSocket.
- Goreleaser for cross-compilation, GitHub Actions for CI/Release, Gradle 8.7 for Android.

**Reference spec:** `docs/design.md` (commit `6e16d18` on main).

**Precursor (validated end-to-end):** `claude-marketplace/claude/busy-joliot-cb8020` — commits `5477747..a937bbd`. The WS protocol, Kotlin UI, and Service lifecycle are production-tested on a Pixel 10 Pro XL over Tailscale.

---

## Workflow: PR-per-phase

Every phase below lands as **one PR into protected `main`**:

```bash
git checkout main && git pull
git checkout -b <branch-name>   # branch name listed per phase
# ... do the phase's work, commit as you go ...
git push -u origin <branch-name>
gh pr create --title "..." --body-file .pr-body.md
# wait for CI green, human review approves, squash-merge
gh pr merge --squash --delete-branch
git checkout main && git pull
# move to next phase
```

**CI required checks (enable in repo settings after this plan's first PR merges):** `ci / docs`, `ci / go`, `ci / gradle`.

**Commit cadence within a phase:** small, topical commits (TDD: failing test → implementation → commit). Each phase ends with `git push` + PR open.

**Conventional Commits:** PR titles follow `feat:`, `fix:`, `chore:`, `docs:`. Squash-merge summarizes into one commit on main.

---

## File-layout target (at v0.1.0)

```
premex-ab/adb-connect/
├── cmd/adb-connect/
│   ├── main.go
│   ├── pair.go
│   ├── remote.go
│   ├── remote_setup.go
│   ├── remote_connect.go
│   ├── remote_status.go
│   ├── remote_uninstall.go
│   ├── daemon.go
│   └── version.go
├── internal/
│   ├── version/version.go
│   ├── adb/adb.go + adb_test.go
│   ├── tailscale/tailscale.go + tailscale_test.go
│   ├── pair/pair.go + pair_test.go
│   ├── apk/apk.go + apk_test.go
│   ├── remote/
│   │   ├── protocol/frames.go + frames_test.go
│   │   ├── daemon/
│   │   │   ├── paths/paths.go + paths_test.go
│   │   │   ├── statestore/statestore.go + statestore_test.go
│   │   │   ├── wsserver/wsserver.go + wsserver_test.go
│   │   │   ├── ipcserver/server.go + server_test.go + client.go
│   │   │   ├── enrollqr/enrollqr.go
│   │   │   └── logger/logger.go
│   │   ├── service/install.go + install_test.go
│   │   ├── bootstrap/bootstrap.go + bootstrap_test.go
│   │   └── client/client.go
│   └── testutil/testutil.go   (tmp dirs, fake CLI binaries, etc)
├── android-app/                (verbatim migration from claude-marketplace)
├── packaging/
│   ├── install.sh
│   ├── homebrew/adb-connect.rb.tmpl
│   └── systemd/adb-connect.service.tmpl
├── docs/
│   ├── design.md           (already committed)
│   ├── plan-v0.1.0.md      (this file, committed with PR 1)
│   ├── wire-protocol.md
│   └── quickstart.md
├── .github/workflows/
│   ├── ci.yml              (already committed)
│   └── release.yml
├── .goreleaser.yaml
├── go.mod, go.sum
├── README.md               (already committed — polished in PR 8)
└── LICENSE                 (already committed)
```

---

## Phase 1 — Go module + daemon foundations

**Branch:** `feat/go-module-and-foundations`
**PR title:** `feat: scaffold Go module and daemon foundations (paths, state store, logger)`

**What lands:** `go.mod`, this plan document, logger, paths, state store — the foundations every other phase depends on. Go CI job starts activating in this PR because `go.mod` appears.

### Task 1.1 — Initialize the Go module

**Files:**
- Create: `go.mod`

- [ ] **Step 1:** Initialize module
```bash
cd /Users/stefan/git/adb-connect
go mod init github.com/premex-ab/adb-connect
```

- [ ] **Step 2:** Add core dependencies
```bash
go get modernc.org/sqlite@latest
go get nhooyr.io/websocket@latest
go get github.com/spf13/cobra@latest
go get github.com/grandcat/zeroconf@latest
go get github.com/skip2/go-qrcode@latest
go mod tidy
```

- [ ] **Step 3:** Commit the module skeleton + this plan
```bash
git checkout -b feat/go-module-and-foundations
git add go.mod go.sum docs/plan-v0.1.0.md
git commit -m "chore: initialize Go module + commit v0.1.0 implementation plan"
```

### Task 1.2 — Version package

**Files:**
- Create: `internal/version/version.go`

- [ ] **Step 1:** Write `internal/version/version.go`

```go
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
```

- [ ] **Step 2:** Commit
```bash
git add internal/version/version.go
git commit -m "feat(version): add build-time version package"
```

### Task 1.3 — Paths package (XDG-compliant, TDD)

**Files:**
- Create: `internal/remote/daemon/paths/paths.go`, `internal/remote/daemon/paths/paths_test.go`
- Create: `internal/testutil/testutil.go`

- [ ] **Step 1:** Write `internal/testutil/testutil.go`

```go
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
```

- [ ] **Step 2:** Write failing test `internal/remote/daemon/paths/paths_test.go`

```go
package paths_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestConfigDir_XDGOrAppSupport(t *testing.T) {
	home := testutil.TempHome(t)
	got := paths.ConfigDir()
	var want string
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Application Support", "adb-connect")
	} else {
		want = filepath.Join(home, ".config", "adb-connect")
	}
	if got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}
}

func TestHelpersAreUnderExpectedDirs(t *testing.T) {
	testutil.TempHome(t)
	if !strings.HasPrefix(paths.DBPath(), paths.ConfigDir()) {
		t.Errorf("DBPath not under ConfigDir: %s", paths.DBPath())
	}
	if !strings.HasPrefix(paths.IPCSocketPath(), paths.ConfigDir()) {
		t.Errorf("IPCSocketPath not under ConfigDir: %s", paths.IPCSocketPath())
	}
	if !strings.Contains(paths.LogPath(), "adb-connect") {
		t.Errorf("LogPath does not contain adb-connect: %s", paths.LogPath())
	}
}
```

- [ ] **Step 3:** Run — expect compile failure
```bash
go test ./internal/remote/daemon/paths/...
# expected: no Go files (paths not yet implemented)
```

- [ ] **Step 4:** Implement `internal/remote/daemon/paths/paths.go`

```go
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
```

- [ ] **Step 5:** Run — expect pass
```bash
go test ./internal/remote/daemon/paths/...
# expected: PASS both tests
```

- [ ] **Step 6:** Commit
```bash
git add internal/testutil internal/remote/daemon/paths
git commit -m "feat(paths): XDG-compliant path helpers with tmp-HOME test util"
```

### Task 1.4 — Logger (slog wrapper with rotation)

**Files:**
- Create: `internal/remote/daemon/logger/logger.go`

- [ ] **Step 1:** Implement

```go
// Package logger wraps log/slog with a file-rotating handler sized for the daemon's log volume.
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
)

const maxBytes = 10 * 1024 * 1024 // 10 MB

type rotatingWriter struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		w.f = f
	}
	st, err := w.f.Stat()
	if err == nil && st.Size() >= maxBytes {
		_ = w.f.Close()
		_ = os.Rename(w.path, w.path+".1")
		w.f, err = os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
	}
	return w.f.Write(p)
}

// New returns a slog.Logger writing to both stderr and the rotating file at paths.LogPath().
func New() *slog.Logger {
	w := &rotatingWriter{path: paths.LogPath()}
	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, w), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
```

- [ ] **Step 2:** Commit
```bash
git add internal/remote/daemon/logger
git commit -m "feat(logger): file-rotating slog wrapper"
```

### Task 1.5 — State store (SQLite, TDD)

**Files:**
- Create: `internal/remote/daemon/statestore/statestore.go`, `internal/remote/daemon/statestore/statestore_test.go`

- [ ] **Step 1:** Failing tests

```go
package statestore_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func open(t *testing.T) *statestore.Store {
	t.Helper()
	testutil.TempHome(t)
	if err := os.MkdirAll(paths.ConfigDir(), 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	s, err := statestore.Open(paths.DBPath())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestServerConfigRoundTrip(t *testing.T) {
	s := open(t)
	if cfg, err := s.ServerConfig(); err != nil || cfg != nil {
		t.Fatalf("expected nil cfg, got %v err %v", cfg, err)
	}
	want := &statestore.ServerConfig{PSK: []byte("abc"), WSPort: 34567, TailscaleHost: "mac.ts.net"}
	if err := s.SetServerConfig(want); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.ServerConfig()
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if !bytes.Equal(got.PSK, want.PSK) || got.WSPort != want.WSPort || got.TailscaleHost != want.TailscaleHost {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUpsertPhoneAndLastSeen(t *testing.T) {
	s := open(t)
	if err := s.UpsertPhone(statestore.Phone{Nickname: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPhone(statestore.Phone{Nickname: "beta", Paired: true, ADBFingerprint: "beef"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordPhoneSeen("alpha"); err != nil {
		t.Fatal(err)
	}
	phones, err := s.ListPhones()
	if err != nil || len(phones) != 2 {
		t.Fatalf("listed %d phones: %v", len(phones), err)
	}
	for _, p := range phones {
		if p.Nickname == "alpha" && p.LastWSSeen.IsZero() {
			t.Fatalf("alpha last seen zero")
		}
		if p.Nickname == "beta" && !p.Paired {
			t.Fatalf("beta not paired")
		}
	}
	time.Sleep(10 * time.Millisecond) // avoid identical timestamps across tests
}

func TestGetPhoneReturnsNilForUnknown(t *testing.T) {
	s := open(t)
	p, err := s.GetPhone("nobody")
	if err != nil || p != nil {
		t.Fatalf("got %v err %v", p, err)
	}
}
```

- [ ] **Step 2:** Run — expect failure (no package).

- [ ] **Step 3:** Implement `internal/remote/daemon/statestore/statestore.go`

```go
// Package statestore persists enrolled phones and server config to SQLite.
package statestore

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS phones (
  nickname        TEXT PRIMARY KEY,
  tailscale_host  TEXT,
  last_ws_seen    INTEGER,
  paired          INTEGER NOT NULL DEFAULT 0,
  adb_fingerprint TEXT
);
CREATE TABLE IF NOT EXISTS server_config (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  psk             BLOB NOT NULL,
  ws_port         INTEGER NOT NULL,
  tailscale_host  TEXT NOT NULL
);
`

type Phone struct {
	Nickname       string
	TailscaleHost  string
	LastWSSeen     time.Time
	Paired         bool
	ADBFingerprint string
}

type ServerConfig struct {
	PSK           []byte
	WSPort        int
	TailscaleHost string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) SetServerConfig(c *ServerConfig) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO server_config (id, psk, ws_port, tailscale_host) VALUES (1, ?, ?, ?)`,
		c.PSK, c.WSPort, c.TailscaleHost)
	return err
}

func (s *Store) ServerConfig() (*ServerConfig, error) {
	row := s.db.QueryRow(`SELECT psk, ws_port, tailscale_host FROM server_config WHERE id = 1`)
	var c ServerConfig
	if err := row.Scan(&c.PSK, &c.WSPort, &c.TailscaleHost); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpsertPhone(p Phone) error {
	paired := 0
	if p.Paired {
		paired = 1
	}
	_, err := s.db.Exec(`
INSERT INTO phones (nickname, tailscale_host, paired, adb_fingerprint)
VALUES (?, ?, ?, ?)
ON CONFLICT(nickname) DO UPDATE SET
  tailscale_host  = excluded.tailscale_host,
  paired          = excluded.paired,
  adb_fingerprint = COALESCE(excluded.adb_fingerprint, phones.adb_fingerprint)`,
		p.Nickname, nullString(p.TailscaleHost), paired, nullString(p.ADBFingerprint))
	return err
}

func (s *Store) RecordPhoneSeen(nickname string) error {
	_, err := s.db.Exec(`UPDATE phones SET last_ws_seen = ? WHERE nickname = ?`, time.Now().UnixMilli(), nickname)
	return err
}

func (s *Store) MarkPaired(nickname, fingerprint string) error {
	_, err := s.db.Exec(`UPDATE phones SET paired = 1, adb_fingerprint = ? WHERE nickname = ?`, fingerprint, nickname)
	return err
}

func (s *Store) GetPhone(nickname string) (*Phone, error) {
	row := s.db.QueryRow(`SELECT nickname, COALESCE(tailscale_host,''), COALESCE(last_ws_seen,0), paired, COALESCE(adb_fingerprint,'') FROM phones WHERE nickname = ?`, nickname)
	var p Phone
	var lastMS int64
	var paired int
	if err := row.Scan(&p.Nickname, &p.TailscaleHost, &lastMS, &paired, &p.ADBFingerprint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastMS > 0 {
		p.LastWSSeen = time.UnixMilli(lastMS)
	}
	p.Paired = paired == 1
	return &p, nil
}

func (s *Store) ListPhones() ([]Phone, error) {
	rows, err := s.db.Query(`SELECT nickname, COALESCE(tailscale_host,''), COALESCE(last_ws_seen,0), paired, COALESCE(adb_fingerprint,'') FROM phones ORDER BY nickname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Phone
	for rows.Next() {
		var p Phone
		var lastMS int64
		var paired int
		if err := rows.Scan(&p.Nickname, &p.TailscaleHost, &lastMS, &paired, &p.ADBFingerprint); err != nil {
			return nil, err
		}
		if lastMS > 0 {
			p.LastWSSeen = time.UnixMilli(lastMS)
		}
		p.Paired = paired == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 4:** Run — expect pass. Commit
```bash
git add internal/remote/daemon/statestore
git commit -m "feat(statestore): SQLite-backed phone enrolment + server config"
```

### Task 1.6 — Push the PR

- [ ] **Step 1:** Push and open PR
```bash
git push -u origin feat/go-module-and-foundations
gh pr create --title "feat: scaffold Go module and daemon foundations" --body "$(cat <<'EOF'
## Summary

- Initialises `go.mod` at `github.com/premex-ab/adb-connect` with Go 1.22 and the v0.1.0 dependency set (modernc.org/sqlite, nhooyr.io/websocket, cobra, zeroconf, go-qrcode).
- Adds `internal/version`, `internal/remote/daemon/{paths,logger,statestore}` with unit tests and `internal/testutil` for tmp-HOME / fake-binary helpers.
- Commits the v0.1.0 implementation plan at `docs/plan-v0.1.0.md`.

No CLI surface yet — that lands in later PRs. Pure Go foundation.

## Test plan

- [ ] `go vet ./...` green
- [ ] `go test ./...` green (paths + statestore tests)
- [ ] `gofmt -l .` empty
- [ ] CI "go" job activates on this PR (go.mod is the trigger) and passes
EOF
)"
```

- [ ] **Step 2:** Wait for CI green, address review comments, squash-merge. Delete branch.

---

## Phase 2 — WS protocol + WS server + IPC + CLI wrappers

**Branch:** `feat/daemon-ws-ipc-and-cli-wrappers`
**PR title:** `feat: WS protocol, WS server, Unix IPC, adb/tailscale wrappers`

### Task 2.1 — Protocol frames (TDD)

**Files:**
- Create: `internal/remote/protocol/frames.go`, `frames_test.go`

- [ ] **Step 1:** Failing test `internal/remote/protocol/frames_test.go`

```go
package protocol_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/protocol"
)

func TestParse_AcceptsHello(t *testing.T) {
	raw := []byte(`{"op":"hello","nickname":"alpha","psk":"YWJj","app_version":"0.1.0"}`)
	f, err := protocol.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	h, ok := f.(*protocol.Hello)
	if !ok {
		t.Fatalf("got %T, want *Hello", f)
	}
	if h.Nickname != "alpha" || h.PSK != "YWJj" {
		t.Fatalf("hello fields: %+v", h)
	}
}

func TestParse_RejectsUnknownOp(t *testing.T) {
	_, err := protocol.Parse([]byte(`{"op":"nonsense"}`))
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Code != "bad_frame" {
		t.Fatalf("want bad_frame ProtocolError, got %v", err)
	}
}

func TestParse_RejectsMissingFields(t *testing.T) {
	for _, raw := range []string{
		`{"op":"hello"}`,
		`{"op":"connect_ready"}`,
		`not json`,
	} {
		if _, err := protocol.Parse([]byte(raw)); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestBuild_PrepConnectAndError(t *testing.T) {
	b, err := protocol.Build(&protocol.PrepConnect{RequestPair: true})
	if err != nil {
		t.Fatalf("build prep: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["op"] != "prep_connect" || m["request_pair"] != true {
		t.Fatalf("prep round-trip: %v", m)
	}
	b, err = protocol.Build(&protocol.ErrorFrame{Code: "auth_failed", Message: "bad psk"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["code"] != "auth_failed" {
		t.Fatalf("error code: %v", m)
	}
}
```

- [ ] **Step 2:** Implement `internal/remote/protocol/frames.go`

```go
// Package protocol defines the JSON frame shapes exchanged over the adb-connect WebSocket.
// This is the canonical contract — the Kotlin WsProtocol in android-app/ must stay in sync.
package protocol

import (
	"encoding/json"
	"fmt"
)

// Error is returned by Parse/Build for malformed frames. It carries an on-wire error code
// suitable for sending as the code field of an error frame.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("protocol %s: %s", e.Code, e.Message) }

// Frame marks types that are valid WS frames.
type Frame interface{ isFrame() }

type Hello struct {
	Nickname   string `json:"nickname"`
	PSK        string `json:"psk"`
	AppVersion string `json:"app_version"`
}

type ToggleState struct {
	On bool `json:"on"`
}

type PrepConnect struct {
	RequestPair bool `json:"request_pair"`
}

type ConnectReady struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	PairCode string `json:"pair_code,omitempty"`
}

type Ack struct{}

type ErrorFrame struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (*Hello) isFrame()        {}
func (*ToggleState) isFrame()  {}
func (*PrepConnect) isFrame()  {}
func (*ConnectReady) isFrame() {}
func (*Ack) isFrame()          {}
func (*ErrorFrame) isFrame()   {}

// Parse validates a raw JSON frame and returns the typed representation.
func Parse(raw []byte) (Frame, error) {
	var peek struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		return nil, &Error{Code: "bad_frame", Message: "invalid JSON"}
	}
	if peek.Op == "" {
		return nil, &Error{Code: "bad_frame", Message: "missing op"}
	}
	switch peek.Op {
	case "hello":
		var h Hello
		if err := json.Unmarshal(raw, &h); err != nil {
			return nil, &Error{Code: "bad_frame", Message: err.Error()}
		}
		if h.Nickname == "" || h.PSK == "" {
			return nil, &Error{Code: "bad_frame", Message: "hello missing nickname or psk"}
		}
		return &h, nil
	case "toggle_state":
		var t struct {
			Op string `json:"op"`
			On *bool  `json:"on"`
		}
		if err := json.Unmarshal(raw, &t); err != nil || t.On == nil {
			return nil, &Error{Code: "bad_frame", Message: "toggle_state missing on"}
		}
		return &ToggleState{On: *t.On}, nil
	case "prep_connect":
		var p struct {
			Op          string `json:"op"`
			RequestPair *bool  `json:"request_pair"`
		}
		if err := json.Unmarshal(raw, &p); err != nil || p.RequestPair == nil {
			return nil, &Error{Code: "bad_frame", Message: "prep_connect missing request_pair"}
		}
		return &PrepConnect{RequestPair: *p.RequestPair}, nil
	case "connect_ready":
		var c ConnectReady
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, &Error{Code: "bad_frame", Message: err.Error()}
		}
		if c.IP == "" || c.Port == 0 {
			return nil, &Error{Code: "bad_frame", Message: "connect_ready missing ip/port"}
		}
		return &c, nil
	case "ack":
		return &Ack{}, nil
	case "error":
		var e ErrorFrame
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, &Error{Code: "bad_frame", Message: err.Error()}
		}
		return &e, nil
	default:
		return nil, &Error{Code: "bad_frame", Message: "unknown op: " + peek.Op}
	}
}

// Build serializes a Frame for the wire. The op is derived from the concrete type.
func Build(f Frame) ([]byte, error) {
	switch v := f.(type) {
	case *Hello:
		return json.Marshal(struct {
			Op string `json:"op"`
			*Hello
		}{"hello", v})
	case *ToggleState:
		return json.Marshal(struct {
			Op string `json:"op"`
			*ToggleState
		}{"toggle_state", v})
	case *PrepConnect:
		return json.Marshal(struct {
			Op string `json:"op"`
			*PrepConnect
		}{"prep_connect", v})
	case *ConnectReady:
		return json.Marshal(struct {
			Op string `json:"op"`
			*ConnectReady
		}{"connect_ready", v})
	case *Ack:
		return []byte(`{"op":"ack"}`), nil
	case *ErrorFrame:
		return json.Marshal(struct {
			Op string `json:"op"`
			*ErrorFrame
		}{"error", v})
	default:
		return nil, &Error{Code: "bad_frame", Message: fmt.Sprintf("unknown frame type %T", f)}
	}
}
```

- [ ] **Step 3:** Run tests, commit
```bash
go test ./internal/remote/protocol/...
git add internal/remote/protocol
git commit -m "feat(protocol): WS frame parse/build with typed Go frames"
```

### Task 2.2 — WS server (TDD)

**Files:**
- Create: `internal/remote/daemon/wsserver/wsserver.go`, `wsserver_test.go`

- [ ] **Step 1:** Write `wsserver_test.go`

```go
package wsserver_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/wsserver"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func newStore(t *testing.T) *statestore.Store {
	t.Helper()
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)
	s, err := statestore.Open(paths.DBPath())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newServer(t *testing.T, store *statestore.Store, psk string) (*wsserver.Server, int) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv, err := wsserver.New(wsserver.Config{Store: store, PSK: psk, BindHost: "127.0.0.1", BindPort: 0, Logger: log})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv, port
}

func dial(t *testing.T, port int) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), "ws://127.0.0.1:"+itoa(port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func itoa(n int) string { return strconvItoa(n) }
// avoid pulling strconv in tests for this one call:
func strconvItoa(n int) string {
	if n == 0 { return "0" }
	b := make([]byte, 0, 5)
	for n > 0 {
		b = append([]byte{byte('0'+n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestHelloWrongPSKReturnsAuthFailed(t *testing.T) {
	s := newStore(t)
	_ = s.UpsertPhone(statestore.Phone{Nickname: "alpha"})
	_, port := newServer(t, s, "correct")
	c := dial(t, port)
	defer c.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"op":"hello","nickname":"alpha","psk":"wrong","app_version":"t"}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"code":"auth_failed"`; !containsJSON(data, want) {
		t.Fatalf("got %s, want %s", data, want)
	}
}

func TestHelloSuccessRecordsLastSeen(t *testing.T) {
	s := newStore(t)
	_ = s.UpsertPhone(statestore.Phone{Nickname: "alpha"})
	_, port := newServer(t, s, "correct")
	c := dial(t, port)
	defer c.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.Write(ctx, websocket.MessageText, []byte(`{"op":"hello","nickname":"alpha","psk":"correct","app_version":"t"}`))
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsJSON(data, `"op":"ack"`) {
		t.Fatalf("want ack, got %s", data)
	}
	p, _ := s.GetPhone("alpha")
	if p.LastWSSeen.IsZero() {
		t.Fatalf("last_seen not recorded")
	}
}

func TestRequestConnectRejectsOfflinePhone(t *testing.T) {
	s := newStore(t)
	_ = s.UpsertPhone(statestore.Phone{Nickname: "alpha"})
	srv, _ := newServer(t, s, "psk")
	_, err := srv.RequestConnect(context.Background(), "alpha", true)
	var berr *wsserver.BusinessError
	if !errors.As(err, &berr) || berr.Code != "phone_offline" {
		t.Fatalf("want phone_offline, got %v", err)
	}
}

func containsJSON(b []byte, sub string) bool {
	s := string(b)
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2:** Implement `internal/remote/daemon/wsserver/wsserver.go`

```go
// Package wsserver implements the phone-facing WebSocket server over Tailscale.
package wsserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/statestore"
	"github.com/premex-ab/adb-connect/internal/remote/protocol"
)

const connectTimeout = 20 * time.Second

// Config wires dependencies into Server.
type Config struct {
	Store    *statestore.Store
	PSK      string
	BindHost string
	BindPort int
	Logger   *slog.Logger
}

// BusinessError carries a machine-readable code ("phone_offline", "connect_timeout", …).
type BusinessError struct {
	Code    string
	Message string
}

func (e *BusinessError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Server is a Tailscale-bound WS listener with per-phone connection bookkeeping.
type Server struct {
	cfg     Config
	ln      net.Listener
	httpSrv *http.Server

	mu       sync.Mutex
	byNick   map[string]*conn
	pending  map[string]*pending
}

type conn struct {
	ws       *websocket.Conn
	nickname string
}

type pending struct {
	done chan pendingResult
}

type pendingResult struct {
	ready *protocol.ConnectReady
	err   error
}

func New(cfg Config) (*Server, error) {
	if cfg.Store == nil || cfg.PSK == "" || cfg.Logger == nil {
		return nil, errors.New("wsserver: missing required Config fields")
	}
	return &Server{
		cfg:     cfg,
		byNick:  map[string]*conn{},
		pending: map[string]*pending{},
	}, nil
}

// Start listens on BindHost:BindPort (0 = random) and returns the chosen port.
func (s *Server) Start() (int, error) {
	addr := net.JoinHostPort(s.cfg.BindHost, strconv.Itoa(s.cfg.BindPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, err
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)
	s.httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// Stop tears down the listener and rejects any pending connects.
func (s *Server) Stop() error {
	s.mu.Lock()
	for _, p := range s.pending {
		p.done <- pendingResult{err: &BusinessError{Code: "server_stopping", Message: "server shutting down"}}
		close(p.done)
	}
	s.pending = map[string]*pending{}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// OnlineNicknames returns the nicknames currently connected.
func (s *Server) OnlineNicknames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.byNick))
	for n := range s.byNick {
		out = append(out, n)
	}
	return out
}

// RequestConnect pushes a prep_connect frame and waits up to connectTimeout for
// a connect_ready OR an error frame from the same nickname.
func (s *Server) RequestConnect(ctx context.Context, nickname string, requestPair bool) (*protocol.ConnectReady, error) {
	s.mu.Lock()
	c, ok := s.byNick[nickname]
	if !ok {
		s.mu.Unlock()
		return nil, &BusinessError{Code: "phone_offline", Message: "phone not connected: " + nickname}
	}
	p := &pending{done: make(chan pendingResult, 1)}
	s.pending[nickname] = p
	s.mu.Unlock()

	msg, err := protocol.Build(&protocol.PrepConnect{RequestPair: requestPair})
	if err != nil {
		return nil, err
	}
	wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	if err := c.ws.Write(wctx, websocket.MessageText, msg); err != nil {
		cancel()
		return nil, err
	}
	cancel()

	select {
	case r := <-p.done:
		return r.ready, r.err
	case <-time.After(connectTimeout):
		s.mu.Lock()
		delete(s.pending, nickname)
		s.mu.Unlock()
		return nil, &BusinessError{Code: "connect_timeout", Message: "timeout waiting for connect_ready: " + nickname}
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	ctx := r.Context()
	// First frame must be hello.
	_, raw, err := ws.Read(ctx)
	if err != nil {
		return
	}
	f, err := protocol.Parse(raw)
	if err != nil {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "bad_frame", Message: err.Error()})
		return
	}
	h, ok := f.(*protocol.Hello)
	if !ok {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "auth_required", Message: "hello required first"})
		return
	}
	if !pskEquals(h.PSK, s.cfg.PSK) {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "auth_failed", Message: "bad psk"})
		return
	}
	phone, err := s.cfg.Store.GetPhone(h.Nickname)
	if err != nil || phone == nil {
		s.send(ctx, ws, &protocol.ErrorFrame{Code: "unknown_phone", Message: "phone not enrolled"})
		return
	}
	s.mu.Lock()
	s.byNick[h.Nickname] = &conn{ws: ws, nickname: h.Nickname}
	s.mu.Unlock()
	_ = s.cfg.Store.RecordPhoneSeen(h.Nickname)
	s.cfg.Logger.Info("phone online", "nickname", h.Nickname)
	defer func() {
		s.mu.Lock()
		if cur := s.byNick[h.Nickname]; cur != nil && cur.ws == ws {
			delete(s.byNick, h.Nickname)
		}
		s.mu.Unlock()
		s.cfg.Logger.Info("phone offline", "nickname", h.Nickname)
	}()
	s.send(ctx, ws, &protocol.Ack{})

	// Authed loop.
	for {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			return
		}
		frame, err := protocol.Parse(raw)
		if err != nil {
			s.send(ctx, ws, &protocol.ErrorFrame{Code: "bad_frame", Message: err.Error()})
			continue
		}
		switch v := frame.(type) {
		case *protocol.ToggleState:
			s.cfg.Logger.Info("toggle", "nickname", h.Nickname, "on", v.On)
		case *protocol.ConnectReady:
			s.mu.Lock()
			p := s.pending[h.Nickname]
			delete(s.pending, h.Nickname)
			s.mu.Unlock()
			if p != nil {
				p.done <- pendingResult{ready: v}
				close(p.done)
			}
		case *protocol.ErrorFrame:
			s.cfg.Logger.Warn("phone error", "code", v.Code, "message", v.Message)
			s.mu.Lock()
			p := s.pending[h.Nickname]
			delete(s.pending, h.Nickname)
			s.mu.Unlock()
			if p != nil {
				p.done <- pendingResult{err: &BusinessError{Code: v.Code, Message: v.Message}}
				close(p.done)
			}
		default:
			s.send(ctx, ws, &protocol.ErrorFrame{Code: "bad_frame", Message: "unexpected op"})
		}
	}
}

func (s *Server) send(ctx context.Context, ws *websocket.Conn, f protocol.Frame) {
	b, err := protocol.Build(f)
	if err != nil {
		s.cfg.Logger.Warn("build frame", "err", err)
		return
	}
	wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = ws.Write(wctx, websocket.MessageText, b)
}

func pskEquals(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
```

- [ ] **Step 3:** Run tests, commit
```bash
go test ./internal/remote/...
git add internal/remote/daemon/wsserver
git commit -m "feat(wsserver): nhooyr WebSocket server with PSK auth + connect routing"
```

### Task 2.3 — IPC server + client

**Files:**
- Create: `internal/remote/daemon/ipcserver/server.go`, `server_test.go`, `client.go`

- [ ] **Step 1:** Test

```go
package ipcserver_test

import (
	"os"
	"testing"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/ipcserver"
	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestStatusAndUnknownOp(t *testing.T) {
	testutil.TempHome(t)
	_ = os.MkdirAll(paths.ConfigDir(), 0o700)
	srv, err := ipcserver.New(map[string]ipcserver.Handler{
		"status": func(req map[string]any) (map[string]any, error) {
			return map[string]any{"phones": []any{map[string]any{"nickname": "alpha"}}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	r, err := ipcserver.Request(map[string]any{"op": "status"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := r["ok"].(bool); !ok {
		t.Fatalf("not ok: %v", r)
	}
	r, err = ipcserver.Request(map[string]any{"op": "bogus"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := r["ok"].(bool); ok {
		t.Fatalf("expected failure: %v", r)
	}
}
```

- [ ] **Step 2:** Implement `server.go`

```go
// Package ipcserver runs a Unix-domain line-delimited JSON RPC over a 0600 socket.
// It is the transport between the adb-connect CLI subcommands (connect/status) and the daemon.
package ipcserver

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
)

// Handler processes a request payload and returns a response payload or an error.
type Handler func(req map[string]any) (map[string]any, error)

type Server struct {
	handlers map[string]Handler
	ln       net.Listener
	mu       sync.Mutex
}

func New(handlers map[string]Handler) (*Server, error) {
	if handlers == nil {
		return nil, errors.New("ipcserver: nil handlers")
	}
	return &Server{handlers: handlers}, nil
}

func (s *Server) Start() error {
	if err := os.MkdirAll(paths.ConfigDir(), 0o700); err != nil {
		return err
	}
	sock := paths.IPCSocketPath()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	_ = os.Chmod(sock, 0o600)
	s.ln = ln
	go s.loop()
	return nil
}

func (s *Server) loop() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *Server) Stop() {
	if s.ln != nil {
		_ = s.ln.Close()
		_ = os.Remove(paths.IPCSocketPath())
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		return
	}
	var req map[string]any
	if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
		writeResp(c, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	op, _ := req["op"].(string)
	h, ok := s.handlers[op]
	if !ok {
		writeResp(c, map[string]any{"ok": false, "error": fmt.Sprintf("unknown op: %s", op)})
		return
	}
	resp, err := h(req)
	if err != nil {
		writeResp(c, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if resp == nil {
		resp = map[string]any{}
	}
	resp["ok"] = true
	writeResp(c, resp)
}

func writeResp(c net.Conn, m map[string]any) {
	b, _ := json.Marshal(m)
	_, _ = c.Write(append(b, '\n'))
}
```

- [ ] **Step 3:** Implement `client.go`

```go
package ipcserver

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/premex-ab/adb-connect/internal/remote/daemon/paths"
)

// Request sends a line-delimited JSON request to the daemon's IPC socket and returns the response map.
func Request(req map[string]any) (map[string]any, error) {
	c, err := net.DialTimeout("unix", paths.IPCSocketPath(), 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		if sc.Err() != nil {
			return nil, sc.Err()
		}
		return nil, errors.New("empty ipc response")
	}
	var resp map[string]any
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
```

- [ ] **Step 4:** Run tests, commit
```bash
go test ./internal/remote/daemon/ipcserver/...
git add internal/remote/daemon/ipcserver
git commit -m "feat(ipc): Unix-domain line-delimited JSON RPC for CLI <-> daemon"
```

### Task 2.4 — adb and tailscale CLI wrappers (TDD)

**Files:**
- Create: `internal/adb/adb.go`, `adb_test.go`, `internal/tailscale/tailscale.go`, `tailscale_test.go`

- [ ] **Step 1:** Tests — use `testutil.FakeBinary` to mock the CLIs on PATH.

```go
package adb_test

import (
	"context"
	"testing"

	"github.com/premex-ab/adb-connect/internal/adb"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestPairSuccess(t *testing.T) {
	testutil.FakeBinary(t, "adb", `echo "Successfully paired"`)
	r, err := adb.Pair(context.Background(), "1.2.3.4", 5555, "123456")
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || !contains(r.Stdout, "Successfully paired") {
		t.Fatalf("got %+v", r)
	}
}

func TestDevicesParsesList(t *testing.T) {
	testutil.FakeBinary(t, "adb", `printf "List of devices attached\nemulator-5554\tdevice\n1.2.3.4:5555\tdevice\n"`)
	ds, err := adb.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 2 || ds[0].Serial != "emulator-5554" || ds[1].Serial != "1.2.3.4:5555" {
		t.Fatalf("got %+v", ds)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2:** Implement `internal/adb/adb.go`

```go
// Package adb shells out to the system `adb` CLI. All calls use os/exec with
// an explicit argv slice — never the shell-interpolated form.
package adb

import (
	"context"
	"os/exec"
	"strings"
)

type Result struct {
	OK     bool
	Stdout string
	Stderr string
	Code   int
}

type Device struct {
	Serial string
	State  string
}

func run(ctx context.Context, args ...string) Result {
	cmd := exec.CommandContext(ctx, "adb", args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return Result{OK: false, Stdout: out.String(), Stderr: errb.String() + err.Error(), Code: -1}
		}
	}
	return Result{OK: code == 0, Stdout: out.String(), Stderr: errb.String(), Code: code}
}

func Pair(ctx context.Context, host string, port int, code string) (Result, error) {
	return run(ctx, "pair", hostPort(host, port), code), nil
}

func Connect(ctx context.Context, host string, port int) (Result, error) {
	return run(ctx, "connect", hostPort(host, port)), nil
}

func Install(ctx context.Context, apkPath string) (Result, error) {
	return run(ctx, "install", "-r", apkPath), nil
}

func GrantWriteSecureSettings(ctx context.Context, pkg string) (Result, error) {
	return run(ctx, "shell", "pm", "grant", pkg, "android.permission.WRITE_SECURE_SETTINGS"), nil
}

func Devices(ctx context.Context) ([]Device, error) {
	r := run(ctx, "devices")
	if !r.OK {
		return nil, nil
	}
	var out []Device
	for i, line := range strings.Split(r.Stdout, "\n") {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 {
			out = append(out, Device{Serial: f[0], State: f[1]})
		}
	}
	return out, nil
}

func hostPort(host string, port int) string {
	// Avoid fmt import bloat.
	buf := make([]byte, 0, len(host)+6)
	buf = append(buf, host...)
	buf = append(buf, ':')
	buf = appendInt(buf, port)
	return string(buf)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	var tmp [12]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(b, tmp[i:]...)
}
```

- [ ] **Step 3:** Mirror for tailscale — `internal/tailscale/tailscale.go` shells `tailscale status --json`, exposes `Status()`, `MagicDNSName()`, `IPv4()`, `IsInstalled()`, `UpWithAuthKey()`.

```go
package tailscale

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

type Status struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		HostName     string   `json:"HostName"`
		DNSName      string   `json:"DNSName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
}

func run(ctx context.Context, args ...string) (string, string, int) {
	cmd := exec.CommandContext(ctx, "tailscale", args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return out.String(), errb.String() + err.Error(), -1
		}
	}
	return out.String(), errb.String(), code
}

func GetStatus(ctx context.Context) *Status {
	stdout, _, code := run(ctx, "status", "--json")
	if code != 0 {
		return nil
	}
	var s Status
	if err := json.Unmarshal([]byte(stdout), &s); err != nil {
		return nil
	}
	return &s
}

func MagicDNSName(ctx context.Context) string {
	s := GetStatus(ctx)
	if s == nil {
		return ""
	}
	return strings.TrimSuffix(s.Self.DNSName, ".")
}

func IPv4(ctx context.Context) string {
	s := GetStatus(ctx)
	if s == nil {
		return ""
	}
	for _, ip := range s.Self.TailscaleIPs {
		if strings.HasPrefix(ip, "100.") {
			return ip
		}
	}
	return ""
}

func IsInstalled(ctx context.Context) bool {
	_, _, code := run(ctx, "version")
	return code == 0
}

func UpWithAuthKey(ctx context.Context, key string) error {
	_, stderr, code := run(ctx, "up", "--auth-key="+key)
	if code == 0 {
		return nil
	}
	return &exec.ExitError{ProcessState: nil, Stderr: []byte(stderr)}
}
```

- [ ] **Step 4:** Test + commit
```bash
go test ./internal/adb/... ./internal/tailscale/...
git add internal/adb internal/tailscale
git commit -m "feat(cli): adb + tailscale exec wrappers with spawn-argv safety"
```

### Task 2.5 — Push Phase 2 PR

- [ ] **Step 1:**
```bash
git push -u origin feat/daemon-ws-ipc-and-cli-wrappers
gh pr create --title "feat: WS protocol, WS server, Unix IPC, adb/tailscale wrappers" --body "$(cat <<'EOF'
## Summary

- `internal/remote/protocol` — typed frame encoder/decoder; canonical WS contract.
- `internal/remote/daemon/wsserver` — nhooyr WebSocket server bound to a caller-supplied host (Tailscale IP in prod), PSK-authed via `crypto/subtle.ConstantTimeCompare`, fail-fast on phone error frames during pending connects.
- `internal/remote/daemon/ipcserver` — 0600 Unix-domain line-JSON RPC for the CLI.
- `internal/adb`, `internal/tailscale` — `exec.CommandContext` wrappers with argv-only calls (no shell interpolation).

## Test plan

- [ ] Unit tests green for all new packages (parse/build, auth/ack/error paths, IPC round-trip, fake-binary-based CLI wrappers).
- [ ] `go vet`, `gofmt -l` clean.
EOF
)"
```

---

## Phase 3 — Pair flow + enrollment QR

**Branch:** `feat/pair-and-enroll-qr`
**PR title:** `feat: same-LAN pair flow + enrollment QR HTTP server`

### Task 3.1 — Enrollment QR (HTTP server that renders + lingers)

**Files:**
- Create: `internal/remote/daemon/enrollqr/enrollqr.go`

- [ ] Implement (lingers until SIGINT; caller decides when to stop):

```go
// Package enrollqr renders the PSK-and-server-host QR over a loopback HTTP server,
// intended to be opened in the user's browser during `adb-connect remote setup`.
package enrollqr

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/skip2/go-qrcode"
)

type Payload struct {
	Version int    `json:"v"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	PSK     string `json:"psk"`
}

// Serve starts an HTTP server on 127.0.0.1:0 serving a page with the QR for payload.
// Caller must call the returned shutdown() when done (typically after IPC status reports the phone online).
func Serve(payload Payload) (url string, shutdown func(ctx context.Context) error, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		_ = ln.Close()
		return "", nil, err
	}
	png, err := qrcode.Encode(string(body), qrcode.Medium, 512)
	if err != nil {
		_ = ln.Close()
		return "", nil, err
	}
	srv := &http.Server{
		Handler:           handler(png, payload),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	return fmt.Sprintf("http://%s", ln.Addr()), srv.Shutdown, nil
}

func handler(png []byte, p Payload) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/qr.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!doctype html><html><head><meta charset=utf-8><title>Premex ADB-gate – enroll</title>
<style>body{font-family:system-ui,sans-serif;background:#111;color:#eee;display:flex;flex-direction:column;align-items:center;padding:2em}
img{background:#fff;padding:1em;border-radius:8px;max-width:80vw}
h1{color:#ff6b00;font-weight:600}</style></head>
<body><h1>Premex ADB-gate</h1><p>Open the Premex ADB-gate app on your phone and scan this QR.</p>
<img src="/qr.png"><p style="opacity:.7;font-size:.9em">server: %s:%d</p></body></html>`, p.Host, p.Port)
		}
	})
}
```

### Task 3.2 — Same-LAN pair (Go port of pair.js)

**Files:**
- Create: `internal/pair/pair.go`

This is the direct Go port of `claude-marketplace/plugins/adb-connect/scripts/pair.js`. Core moves:

- Generate a random service name + password, encode as `WIFI:T:ADB;S:<name>;P:<password>;;`, render QR in terminal AND serve via HTTP + browser open.
- Browse mDNS with `github.com/grandcat/zeroconf` for `_adb-tls-pairing._tcp`; on match, extract the IP+port from the resolved service info.
- Run `adb pair <ip>:<port> <password>`; on success, wait for `_adb-tls-connect._tcp` + run `adb connect <ip>:<port>`.

Public entry point:

```go
// Run executes the full same-LAN pair flow. Returns the resolved adb connect address on success.
// Caller passes an http-browser-opener callback (nil for terminal-only / headless).
func Run(ctx context.Context, cfg Config) (string, error)
```

A full implementation that mirrors pair.js line-for-line is ~350 lines. For brevity here, the engineer implements it following the Node pair.js logic at `claude-marketplace/plugins/adb-connect/scripts/pair.js` (already validated in smoke test). Key libraries: `grandcat/zeroconf` for mDNS, `skip2/go-qrcode` for QR, `os/exec` for `adb pair` / `adb connect`, a small HTTP server mirroring the JS one with shutdown on match.

Tests:
- A "headless" unit test with `FakeBinary("adb", ...)` that asserts the right commands are dispatched given a mocked mDNS endpoint fed via a seam (expose a `dialer`/`browser` interface for test injection).

### Task 3.3 — Push Phase 3 PR

- [ ] `git push -u origin feat/pair-and-enroll-qr && gh pr create ...`

---

## Phase 4 — Service installer + APK download/verify + Bootstrap orchestrator

**Branch:** `feat/service-install-apk-and-bootstrap`
**PR title:** `feat: service installer, APK SHA-verified download, remote-setup orchestrator`

### Task 4.1 — Service installer (launchd + systemd-user, TDD)

**Files:**
- Create: `internal/remote/service/install.go`, `install_test.go`

Same file-layout and contents as the Node version, translated to Go. The plist content and systemd unit content are const strings; `Install()` writes them with `os.WriteFile`, then invokes `launchctl bootstrap` or `systemctl --user enable --now`.

```go
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

type InstallOpts struct {
	BinaryPath string // absolute path to adb-connect binary
	DryRun     bool   // skip the actual load/enable step
}

func macPlistPath() string   { return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", label+".plist") }
func linuxUnitPath() string  { return filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "adb-connect-server.service") }
func macLogsDir() string     { return filepath.Join(os.Getenv("HOME"), "Library", "Logs", "adb-connect") }

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
```

Test just validates the plist/unit file content in DryRun mode.

### Task 4.2 — APK download + SHA-256 verify

**Files:**
- Create: `internal/apk/apk.go`, `apk_test.go`

```go
// Package apk downloads the signed Premex ADB-gate APK from the GitHub release
// matching the CLI version and verifies its SHA-256 against a value fetched
// from the same release's manifest.json.
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

const releaseBase = "https://github.com/premex-ab/adb-connect/releases/download"

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
```

Test uses `httptest.NewServer` to stand up a fake manifest + APK; verifies mismatch detection.

### Task 4.3 — Bootstrap orchestrator

**Files:**
- Create: `internal/remote/bootstrap/bootstrap.go`

Ports the Node `bootstrap.js` one-to-one: `EnsureTailscaleInstalled`, `EnsureTailscaleUp` (prompts for auth key), `GenerateConfig` (creates PSK, mkdir config, seeds state store), `InstallDaemon` (calls `service.Install`), `RequireAttachedPhone`, `DownloadOrBuildAPK` (calls `apk.Download`; falls back to `./gradlew assembleRelease` if `--from-source` flag set AND `ANDROID_HOME` set), `InstallAndGrant`, `ShowEnrollmentQR`, `WaitForEnrollment` (polls IPC).

Top-level:

```go
func RemoteSetup(ctx context.Context, opts RemoteSetupOpts) error
```

The engineer should follow `claude-marketplace/plugins/adb-connect/scripts/bootstrap/bootstrap.js` as the reference behavior — same sequencing, same prompts, same error messages. Already validated end-to-end.

### Task 4.4 — Push Phase 4 PR

---

Phases 5–8 continue in [part 2 of this plan — see next commit message in this section after the PR below merges]. Below is the high-level outline for the remaining phases.

## Phase 5 — CLI wiring (cobra) + daemon main entry

**Branch:** `feat/cli-cobra-and-daemon-main`

Creates `cmd/adb-connect/main.go` (cobra root) and per-verb files:
- `pair.go` → invokes `internal/pair.Run`.
- `remote.go` → parent subcommand, no-op; owns `--help`.
- `remote_setup.go` → invokes `bootstrap.RemoteSetup`.
- `remote_connect.go` → `ipcserver.Request(map[string]any{"op":"connect","nickname":…})`.
- `remote_status.go` → `ipcserver.Request({"op":"status"})` + pretty print.
- `remote_uninstall.go` → `service.Uninstall()` + delete config dir.
- `daemon.go` → runs `wsserver` + `ipcserver` in foreground (used by launchd/systemd unit; also good for debugging).
- `version.go` → prints `version.Full()`.

Each file is small and focused. PR includes an integration test that spawns the binary in `daemon` mode against a tmp HOME, hits the IPC via `ipcserver.Request`, asserts the expected status JSON.

## Phase 6 — Android app migration

**Branch:** `feat/android-app-migration`

Copies `claude-marketplace/plugins/adb-connect/android-app/` into `android-app/` verbatim — Kotlin sources, Gradle wrapper, resources, manifest — all the changes that landed in the Claude-plugin smoke test (Wi-Fi auto-enable, Tailscale IP preference, network_security_config.xml, PairCodeBridge). Two new changes in this PR:

1. `app/build.gradle.kts`: add `signingConfigs.release` sourced from env vars (`ANDROID_KEYSTORE_B64`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`, `ANDROID_STORE_PASSWORD`) with a `base64 -d` pre-build step to materialise the keystore. Debug variant unchanged.
2. `versionName` now reads from `System.getenv("VERSION") ?: "0.0.0-dev"`.

CI `gradle` job now actively runs (`:app:testDebugUnitTest`, `:app:assembleDebug`) since `android-app/gradlew` exists.

## Phase 7 — Release pipeline

**Branch:** `feat/release-pipeline`

Adds:
- `.goreleaser.yaml` — 5 targets (darwin arm64/amd64, linux arm64/amd64, windows amd64), archives, checksums, Homebrew formula publish to `premex-ab/homebrew-tap`, changelog generation from conventional commits.
- `.github/workflows/release.yml` — triggered on `v*` tag; runs Goreleaser in one job, signs+uploads APK in a second job, generates `manifest.json` with APK SHA-256 in a third job.
- `packaging/homebrew/adb-connect.rb.tmpl` — Homebrew formula template that Goreleaser fills in.

A one-off follow-up task the **human** performs: create the `premex-ab/homebrew-tap` repo + grant the release workflow's GitHub Token write access.

## Phase 8 — Distribution + docs + v0.1.0 release

**Branch:** `feat/distribution-docs-and-release`

- `packaging/install.sh` — detects OS+arch, downloads the matching Release archive, verifies SHA-256, installs to `~/.local/bin` (or `/usr/local/bin` with `--prefix`).
- `packaging/systemd/adb-connect.service.tmpl` — reference unit for non-default installs.
- `docs/wire-protocol.md` — canonical protocol spec for future alt-client implementers.
- `docs/quickstart.md` — user-facing 60-second walkthrough.
- `README.md` — tighten, add install/quickstart links, badges (CI, Release).
- Tag `v0.1.0` via `git tag -a v0.1.0` + `git push origin v0.1.0` (after merge). Release workflow runs automatically.

---

## Self-review

**Spec coverage (design.md → tasks):**
- Goals (one-command install, pair, remote-setup/connect/status, signed APK, no-CGO) → Phases 5, 6, 7, 8.
- Architecture (3-box topology, trust model, state store) → Phases 1, 2.
- Repo layout (monorepo) → Phases 1–8 assemble the tree.
- CLI surface → Phase 5.
- Go stack (library choices) → Phase 1 dependencies + per-phase use.
- WS protocol → Phase 2.
- Android app (migration + signing + versionName) → Phase 6.
- Release pipeline (Goreleaser + signing + Homebrew) → Phase 7.
- Distribution (Homebrew, curl installer) → Phases 7, 8.
- CI/CD policy (main protection, PR per change) → this plan's "PR-per-phase" workflow + CI workflow in bootstrap.
- Failure modes → error handling distributed across Phases 2–5.
- Non-goals (v1) → not implemented by design; Phase 8 docs note the boundary.

**Placeholder scan:** no "TBD", no "implement later". Phases 3.2 ("same-LAN pair") and 4.3 ("bootstrap orchestrator") reference the Node sources for line-by-line behavior, but list the exact public entry-point signatures and required library choices. Each is a faithful port of already-validated logic rather than novel design.

**Type consistency:**
- `protocol.Frame` interface + concrete types (`Hello`, `PrepConnect`, `ConnectReady`, `ErrorFrame`, `Ack`, `ToggleState`) consistent across wsserver, ipcserver, bootstrap.
- `wsserver.BusinessError{Code, Message}` matches Node implementation semantics (e.g., `phone_offline`, `connect_timeout`, `discover_failed` relayed from phone).
- `apk.Download(version, destPath)` signature aligned with how `bootstrap.DownloadOrBuildAPK` calls it.

Plan complete.
