# adb-connect

[![CI](https://github.com/premex-ab/adb-connect/actions/workflows/ci.yml/badge.svg)](https://github.com/premex-ab/adb-connect/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/premex-ab/adb-connect)](https://github.com/premex-ab/adb-connect/releases)

One-command wireless `adb` on the same Wi-Fi — no cables, no Android Studio required.

## Install

    brew install premex-ab/tap/adb-connect
    # or
    curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh

## Usage

    adb-connect install-app       # sideload the signed Premex ADB-gate companion app (once per phone)
    adb-connect pair              # QR-pair the phone and add it to adb devices (once per phone)
    adb-connect service install   # install background watcher — phones auto-appear on Wi-Fi toggle
    adb-connect version

See the [quickstart](docs/quickstart.md) for a step-by-step walkthrough.

## How it works

`adb-connect pair` drives the same mDNS-based QR pairing flow that Android Studio uses — it renders a QR code, waits for the phone to scan it via the Wireless Debugging panel, then runs `adb pair` and `adb connect` automatically.

The **Premex ADB-gate** companion Android app (sideloaded by `install-app`) provides a one-tap toggle to enable ADB over Wi-Fi without touching Developer options every time. It holds the `WRITE_SECURE_SETTINGS` permission (granted at install time) so it can toggle wireless debugging programmatically.

`adb-connect service install` registers a background watcher as a launchd user agent (macOS) or systemd-user unit (Linux). The watcher browses `_adb-tls-connect._tcp` on the LAN. When the ADB-gate app toggle turns ON, the phone advertises the service and the laptop automatically runs `adb connect` — the phone appears in `adb devices` within a few seconds, no manual command needed.

## Architecture

See [`docs/design.md`](docs/design.md).

## License

Apache-2.0 — see [`LICENSE`](LICENSE).
