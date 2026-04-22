# adb-connect — design (v0.3.0)

**Status:** Implemented.
**Date:** 2026-04-21 (v0.2.0) / 2026-04-22 (v0.3.0 addendum)

## Summary

`adb-connect` is a CLI tool that gives developers a one-command path to `adb` against an Android phone on the same Wi-Fi network. It ships a companion privileged Android app ("Premex ADB-gate") whose source lives in this repository and whose signed APK is published alongside each CLI release.

## Scope (v0.2.0)

**In scope:**
- `adb-connect pair` — same-LAN QR pairing flow (mDNS + `adb pair` + `adb connect`).
- `adb-connect install-app` — downloads + SHA-verifies + sideloads the signed Premex ADB-gate APK.
- Premex ADB-gate Android companion app — a single-screen toggle that programmatically enables ADB over Wi-Fi.

**Out of scope (dropped from v0.1.x):**
- Tailscale / VPN mesh integration.
- Daemon (`adb-connect daemon`) and its WebSocket server.
- Remote commands (`adb-connect remote setup/connect/status/uninstall`).
- IPC, state store (SQLite), PSK-based auth.

## Architecture

Two components, joined by the same-LAN `adb pair` / `adb connect` protocol:

```
┌──────────────────────────┐    mDNS + adb pair/connect    ┌──────────────────────────┐
│  Developer laptop         │ ────────────────────────────▶ │  Android phone            │
│  adb-connect pair         │                               │  Premex ADB-gate app      │
│  adb-connect install-app  │                               │  (ADB-over-Wi-Fi toggle)  │
└──────────────────────────┘                               └──────────────────────────┘
```

### CLI (`adb-connect`)

Written in Go. No CGO — cross-compiled static binaries for macOS arm64/amd64, Linux arm64/amd64, Windows amd64.

| Command | What it does |
|---|---|
| `pair` | Renders a QR code; user scans from Android's Wireless Debugging panel. Runs `adb pair` + `adb connect` to complete the pairing. |
| `install-app` | Downloads the signed APK from GitHub Releases, verifies SHA-256 against `<apk>.sha256`, then `adb install -r` + `pm grant WRITE_SECURE_SETTINGS`. |
| `watch` | Browses `_adb-tls-connect._tcp` on the LAN and auto-runs `adb connect` for each new paired phone. Blocks until SIGINT/SIGTERM. |
| `service install` | Installs `watch` as a launchd user agent (macOS) or systemd-user unit (Linux) so it starts automatically at login. |
| `service uninstall` | Stops and removes the installed service. |
| `version` | Prints the CLI version. |

### Android app (Premex ADB-gate)

A minimal Kotlin/Compose app with a single screen:

- **Master toggle (Switch):** ON → Wi-Fi auto-enable → `Settings.Global.putInt("adb_wifi_enabled", 1)` → start foreground service. OFF → stop service → `adb_wifi_enabled = 0`.
- **Status line:** discovers the phone's own `_adb-tls-connect._tcp` mDNS service and shows the `IP:port` when ADB is active.
- **Wi-Fi panel prompt:** if Wi-Fi is off, shows a card with a button to open system Wi-Fi settings.
- **Persistent foreground notification:** "ADB-gate active / Wireless debugging enabled" — keeps the service alive and visible to the user.

Requires the `WRITE_SECURE_SETTINGS` permission, granted at install time by `adb-connect install-app`.

## v0.3.0 — mDNS auto-connect watcher

After a phone is paired once with `adb-connect pair`, toggling the Premex ADB-gate app ON causes the phone to advertise `_adb-tls-connect._tcp` on the LAN. The laptop-side `watch` command browses that service, calls `adb connect <ip>:<port>` for each new advertisement, and the phone appears in `adb devices` within a few seconds — no manual command needed.

The recommended setup is:

    adb-connect service install

This registers `adb-connect watch` as a launchd user agent (macOS) or systemd-user unit (Linux) that starts at login and runs continuously in the background. The launchd plist sets an explicit `PATH` covering Homebrew prefixes (`/opt/homebrew/bin`, `/usr/local/bin`, …) so that `adb` is found even though launchd's own default PATH is minimal.

The watcher deduplicates by mDNS instance name and reconciles against `adb devices` every 15 seconds, dropping stale entries so they reconnect on the next announcement.

## Repo layout

```
premex-ab/adb-connect/
├── cmd/adb-connect/
│   ├── main.go           # cobra root (pair, install-app, watch, service, version)
│   ├── pair.go
│   ├── install_app.go
│   ├── watch.go          # `watch` subcommand
│   ├── service.go        # `service install/uninstall` subcommands
│   └── version.go
├── internal/
│   ├── adb/              # adb CLI wrapper (Pair, Connect, Install, Devices, GrantWriteSecureSettings)
│   ├── apk/              # APK download + SHA-256 verify (against <apk>.sha256)
│   ├── pair/             # same-LAN QR pair flow
│   ├── service/          # launchd plist + systemd-user unit installer
│   ├── testutil/
│   ├── version/
│   └── watch/            # mDNS browse loop + adb-connect logic
├── android-app/          # Kotlin + Compose source for the companion app
├── docs/
│   ├── design.md         # this document
│   ├── quickstart.md
│   └── release-setup.md
├── .github/workflows/
│   ├── ci.yml            # go vet, go test, gofmt, gradle debug build
│   └── release.yml       # goreleaser + signed APK + .sha256 upload
├── go.mod, go.sum
└── .goreleaser.yaml
```

## Release pipeline

### CI

- `go vet ./...`
- `go test ./...`
- `gofmt -l` check
- `./gradlew :app:assembleDebug` (smoke APK build)

### Release (on `v*` tag)

1. Goreleaser builds Go binaries, creates GitHub Release, updates Homebrew tap.
2. Android job: `./gradlew :app:assembleRelease` with secrets-sourced keystore.
3. Stages `adb-gate-<ver>.apk` + `adb-gate-<ver>.apk.sha256` and uploads both to the Release.
4. `adb-connect install-app` downloads and verifies against the `.sha256` file at runtime.

## Distribution

- **Homebrew:** `brew install premex-ab/tap/adb-connect` (macOS/Linux).
- **curl:** `curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh`
- **Manual:** GitHub Releases archives.
