# adb-connect — design

**Status:** Accepted — ready for implementation planning.
**Date:** 2026-04-21

## Summary

`adb-connect` is a standalone CLI tool that gives developers — and AI agents — a one-command path to `adb` against an Android phone, whether the phone is on the same Wi-Fi or anywhere reachable by Tailscale. It ships a companion privileged Android app ("Premex ADB-gate") whose source lives in this repository and whose signed APK is published alongside each CLI release.

The project originated as a Claude plugin (claude-marketplace/plugins/adb-connect) and proved the design end-to-end over Tailscale on real hardware. This repository re-implements the CLI + daemon in Go to make the feature trivially installable for human developers via Homebrew and a curl installer, and for AI agents by wrapping through the Claude plugin as a thin shim.

## Goals

- One-command install on macOS and Linux (`brew install premex-ab/tap/adb-connect` or `curl | sh`).
- `adb-connect pair` — same-LAN QR pairing, identical flow to Android Studio's "Pair devices using Wi-Fi".
- `adb-connect remote setup` — end-to-end bootstrap: installs Tailscale if missing, starts a user-level daemon, builds/installs the companion Android app (or `adb install`s the pre-built release APK), grants `WRITE_SECURE_SETTINGS`, shows an enrollment QR.
- `adb-connect remote connect [nickname]` — triggers a remote connection; daemon runs `adb pair`/`adb connect` over the tailnet.
- `adb-connect remote status` — list enrolled phones + online/offline state.
- Signed release APK per version, SHA-256 verified at install time.
- No CGO in the Go binary — static cross-compiled distributables for macOS arm64/amd64, Linux arm64/amd64, Windows amd64.

## Non-goals (v1)

- Windows daemon (Windows binary only runs same-LAN `pair`).
- libtailscale embedding — shell out to the system `tailscale` CLI (same rationale as the Claude-plugin design).
- Multi-tenant or shared-daemon mode.
- Auto-updater.
- Telemetry / usage analytics.
- winget / Scoop packages (Homebrew + curl installer + Releases archives in v1).
- Docker image.
- macOS code-signing / notarization (documented install-script workaround; proper signing is v2 when we have an Apple Developer ID).

## Architecture

Three boxes, same shape as the Claude-plugin design, joined by Tailscale:

```
┌──────────────────────┐        ┌────────────────────┐        ┌────────────────────┐
│  Developer / agent   │  adb   │  Go daemon         │   WSS  │  Premex ADB-gate   │
│  laptop              │──tcp──▶│  (launchd/         │◀──────▶│  app + Tailscale   │
│  (`adb-connect ...`) │        │   systemd-user)    │ tailnet│  app on phone      │
└───────────────────────────────┴────────────────────┘        └────────────────────┘
```

**Trust model (unchanged from Claude-plugin design):**
- Outer: Tailscale ACLs for tailnet membership.
- Inner: 256-bit PSK delivered via scanned QR at app first-launch, validated per WS frame with `crypto/subtle.ConstantTimeCompare`.

**State store:** SQLite at `~/.config/adb-connect/devices.db` (Linux) / `~/Library/Application Support/adb-connect/devices.db` (macOS). Schema: `phones` (nickname PK, tailscale_host, last_ws_seen, paired, adb_fingerprint) and `server_config` (psk, ws_port, tailscale_host).

## Repo layout (monorepo)

```
premex-ab/adb-connect/
├── README.md                           # product README (install, quickstart)
├── LICENSE                             # Apache-2.0
├── .github/
│   ├── workflows/{ci.yml, release.yml, codeql.yml}
│   ├── CODEOWNERS
│   └── pull_request_template.md
├── cmd/adb-connect/main.go             # cobra root
├── internal/
│   ├── adb/                            # `adb` CLI wrapper
│   ├── tailscale/                      # `tailscale` CLI wrapper
│   ├── pair/                           # same-LAN QR pair flow
│   ├── remote/
│   │   ├── daemon/                     # WS server, IPC, state store, enroll QR
│   │   ├── bootstrap/                  # remote-setup orchestrator
│   │   ├── client/                     # IPC client used by CLI verbs
│   │   ├── protocol/                   # WS frame types — canonical contract
│   │   └── service/                    # launchd + systemd-user installer
│   ├── apk/                            # APK download + SHA-256 verify
│   └── version/                        # version string, baked at build time via -ldflags
├── android-app/                        # migrated verbatim from claude-marketplace
│   ├── settings.gradle.kts
│   ├── build.gradle.kts
│   ├── gradle.properties
│   ├── gradle/wrapper/{gradle-wrapper.jar, gradle-wrapper.properties}
│   ├── gradlew, gradlew.bat
│   └── app/src/main/...                # Kotlin + Compose sources
├── docs/
│   ├── design.md                       # this document
│   ├── wire-protocol.md                # canonical WS protocol doc
│   └── quickstart.md
├── packaging/
│   ├── homebrew/adb-connect.rb.tmpl    # published to premex-ab/homebrew-tap
│   ├── install.sh                      # curl | sh installer
│   └── systemd/adb-connect.service.tmpl
├── go.mod, go.sum
├── .goreleaser.yaml
└── .gitignore
```

## CLI surface

```
adb-connect pair                          # same-LAN QR pair flow
adb-connect remote setup                  # one-time bootstrap (install Tailscale + daemon + app + enrol phone)
adb-connect remote connect [nickname]     # trigger remote connection
adb-connect remote status                 # list enrolled phones
adb-connect remote uninstall              # tear down daemon + config
adb-connect daemon                        # run daemon in foreground (used by launchd/systemd unit)
adb-connect version
adb-connect completion {bash,zsh,fish,powershell}
adb-connect help
```

`remote-*` verbs are grouped under a `remote` subcommand for discoverability via `adb-connect remote --help`. Flat `pair` stays flat because it's independent of the remote flow.

## Go stack

| Concern | Library | Reason |
|---|---|---|
| CLI framework | `github.com/spf13/cobra` | Ubiquitous, feature-complete. |
| WebSocket | `nhooyr.io/websocket` | Modern, context-aware, no CGO. |
| SQLite | `modernc.org/sqlite` | Pure Go — avoids CGO for cross-compilation. |
| mDNS | `github.com/grandcat/zeroconf` | Actively maintained DNS-SD client/server. |
| QR | `github.com/skip2/go-qrcode` | De-facto standard. |
| Logging | `log/slog` (stdlib) | Modern, structured, stdlib-only. |
| Test | `testing` (stdlib) + table-driven | No assert library needed. |

No CGO means clean cross-compilation to all five targets without platform-specific toolchains.

## WS protocol

The same JSON frame protocol used by the Node implementation in the Claude plugin, unchanged. The canonical source of truth is `internal/remote/protocol/frames.go` (Go structs with `json:` tags). The Kotlin `WsProtocol.kt` on the Android side stays in sync; any incompatible change gets a protocol version bump and a CLI/APK compat check at connect time.

Frames:
- `hello` (phone → server): `{op, nickname, psk, app_version}`
- `ack` (server → phone)
- `toggle_state` (phone → server): `{op, on}`
- `prep_connect` (server → phone): `{op, request_pair}`
- `connect_ready` (phone → server): `{op, ip, port, pair_code?}`
- `error` (either): `{op, code, message}`

PSK validation uses `crypto/subtle.ConstantTimeCompare`. Error frames during a pending connect fail-fast the daemon's `requestConnect` (lesson from the Node smoke test).

## Android app

Migrated verbatim from `claude-marketplace/plugins/adb-connect/android-app/`. No functional changes. What's new in this repo:

- **Release variant + signing.** A `release` build type signed with a keystore supplied via GitHub Actions secrets (`ANDROID_KEYSTORE_B64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`). Debug variant unchanged (self-signed, usable for local dev).
- **`versionName` from `VERSION` env var.** The release workflow sets `VERSION=${GITHUB_REF_NAME#v}` and the Gradle build picks it up.
- **Version-matched compat check.** CLI reads the installed APK's `versionName` via `adb shell dumpsys` and warns when it mismatches the CLI version by a minor-version or more.

No changes to package id (`se.premex.adbgate`), min/target SDK (30/35), UI, or service behavior.

## Release pipeline

### CI (`.github/workflows/ci.yml`, runs on PRs and main pushes)

- `go vet ./...`
- `go test ./...`
- `staticcheck ./...`
- `./gradlew :app:testDebugUnitTest` (JVM unit tests for Kotlin)
- `./gradlew :app:assembleDebug` (smoke the APK build)
- `gofmt -l` check (no unformatted files)

All three checks are required by branch protection. PRs can't merge without green CI.

### Release (`.github/workflows/release.yml`, runs on `v*` tags)

1. Build Go binaries via Goreleaser for `darwin/arm64`, `darwin/amd64`, `linux/arm64`, `linux/amd64`, `windows/amd64`.
2. Goreleaser creates `.tar.gz` / `.zip` archives, SHA-256 checksums, and a GitHub Release.
3. Android job runs `./gradlew :app:assembleRelease` with secrets-sourced keystore → produces signed APK.
4. SHA-256 of the APK computed; uploaded as `adb-gate-<version>.apk` + `adb-gate-<version>.apk.sha256` on the Release.
5. Goreleaser updates `premex-ab/homebrew-tap/Formula/adb-connect.rb` via its built-in Homebrew publisher.
6. `manifest.json` at `releases/latest/` records the APK SHA-256 so the CLI can fetch-and-verify at runtime.

### Signing key bootstrap (one-time, you-run-it)

```
keytool -genkey -v -keystore release.keystore \
  -alias adbgate -keyalg RSA -keysize 4096 -validity 10950 \
  -storepass <store-pass> -keypass <key-pass> \
  -dname "CN=Premex AB, O=Premex, C=SE"
base64 -i release.keystore -o release.keystore.b64   # paste contents into GitHub Secret
```

Secrets to set in the repo: `ANDROID_KEYSTORE_B64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` (= `adbgate`), `ANDROID_KEY_PASSWORD`.

## Distribution

- **Homebrew:** `brew install premex-ab/tap/adb-connect` for macOS arm64/amd64 + Linux. Goreleaser auto-pushes to `premex-ab/homebrew-tap` on each release.
- **curl installer:** `curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh` — hosted on GitHub Pages from `packaging/install.sh`. Detects OS/arch, downloads the matching archive, verifies SHA-256 against the published checksum file, installs to `~/.local/bin` (or `/usr/local/bin` with `--prefix`).
- **Manual:** GitHub Releases page — pick the platform archive, verify checksum, extract.
- **Windows:** Manual archive install in v1; Windows binary works for `pair` (same-LAN) only (daemon not supported yet).

## CI/CD policy

- Main branch is protected; merges require: green CI, 1 approval, up-to-date branch, no directly-pushed commits.
- Bootstrap PR (opens from `bootstrap` branch) is the exception — it introduces the CI workflow itself. The workflow runs on the PR and becomes the required-check gate for subsequent PRs once the user configures branch protection in repo settings.
- Squash-merge by default.
- Conventional Commits in PR titles (`feat:`, `fix:`, `docs:`, …) — enforced by a lightweight PR-title check in CI (v2 if needed).

## Failure modes handled explicitly

| Situation | Behavior |
|---|---|
| `tailscale` not installed at setup | Install via `brew install tailscale` (macOS) or `curl ... install.sh \| sh` (Linux). Error with install URL if Homebrew absent on macOS. |
| `adb` not on PATH | Error with platform-specific install instruction. |
| No phone attached during `remote setup` | "Connect a phone via USB or run `adb-connect pair` first, then retry." |
| APK download fails | Fall back to `./gradlew assembleRelease` if `ANDROID_HOME` set AND `--from-source` passed; otherwise retry + error. |
| APK SHA-256 mismatches manifest | Abort with clear error. Never install. |
| Daemon already running with different PSK | Detect via IPC, prompt to reset. |
| Phone offline when `connect` runs | `phone_offline` error; "flip the toggle ON in Premex ADB-gate". |
| mDNS discovery fails during connect | Phone sends `discover_failed` error frame; daemon fail-fast propagates it. |
| WS disconnect idle | Phone auto-reconnects with exponential backoff capped at 60 s. |
| CLI version << APK version | Warn with upgrade instruction; continue (best-effort). |
| APK version << CLI version | Error with `adb-connect remote setup --force-reinstall-apk` hint. |

## Migration plan (sub-project 2, deferred)

Once `adb-connect` v0.1.0 is published, a follow-up branch in `claude-marketplace`:

- Deletes `plugins/adb-connect/scripts/server/`, `scripts/bootstrap/`, `android-app/`.
- Rewrites `scripts/pair.js` to a two-line stub that execs `adb-connect pair` (or removes the .js entirely).
- Rewrites `commands/*.md` frontmatter and bodies to invoke `adb-connect <verb>` via the Bash tool.
- Adds a SessionStart hook that runs `brew install premex-ab/tap/adb-connect` (or the curl installer) if the binary is missing — matches the `android-cli` plugin's eager-install pattern.
- Updates README to link at `github.com/premex-ab/adb-connect` as the product.

Resulting plugin is a thin wrapper, ~50 lines of slash-command and hook YAML.

## Open questions deferred to implementation

1. **Homebrew tap repo.** Does `premex-ab/homebrew-tap` already exist? If not, scaffold it as part of sub-project 1 or create during first release. Goreleaser's `brews:` stanza accepts a target repo and creates the first formula.
2. **Apple Developer ID for notarization.** Not required for v1 (users `sudo xattr -d com.apple.quarantine` as a one-time workaround, documented). When available: Goreleaser has a signing stanza.
3. **Go minimum version.** Plan for Go 1.22+ (stable for ~2 years by release time). Downstream consumers of the Homebrew formula inherit whatever Homebrew's Go formula provides.

None of these block the bootstrap PR.

## Testing strategy

- **Unit (Go):** each `internal/*` package with `_test.go`; target ≥80% coverage of frame validation, state-store CRUD, IPC round-trips, APK SHA verification, service-install file-generation.
- **Integration (Go):** spin up the daemon in a tmp dir with mocked `adb`/`tailscale` CLIs on PATH; WS mock client simulates the phone; assert `adb connect` called with expected args.
- **Unit (Kotlin):** `WsProtocol`, `WsBackoff`, `Config.fromQrPayload` — same tests that currently live in the Claude-plugin branch, moved verbatim.
- **CI:** all unit + integration tests + `:app:testDebugUnitTest` + `:app:assembleDebug` block PR merge.
- **Manual smoke** (`docs/manual-smoke.md`): pre-release checklist for the full remote-setup → pair → connect flow on real hardware, over both same-LAN and Tailscale-only paths.

## References

- Claude-plugin precursor (validates architecture on real hardware): `claude-marketplace/docs/superpowers/specs/2026-04-21-adb-connect-remote-tailscale-design.md`
- Original implementation + end-to-end smoke test results: `claude-marketplace/claude/busy-joliot-cb8020` branch, commits `5477747..a937bbd`.
