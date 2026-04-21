# adb-connect

[![CI](https://github.com/premex-ab/adb-connect/actions/workflows/ci.yml/badge.svg)](https://github.com/premex-ab/adb-connect/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/premex-ab/adb-connect)](https://github.com/premex-ab/adb-connect/releases)

Connect `adb` to an Android phone from anywhere — same Wi-Fi, or across networks via [Tailscale](https://tailscale.com/). One command to pair, one command to connect, every time.

## Install

    brew install premex-ab/tap/adb-connect
    # or
    curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh

## Usage

See the [quickstart](docs/quickstart.md) for a 60-second walkthrough.

    adb-connect pair                          # same-Wi-Fi QR flow
    adb-connect remote setup                  # bootstrap Tailscale + daemon + companion app
    adb-connect remote connect [nickname]     # connect to a remote phone
    adb-connect remote status                 # list enrolled phones
    adb-connect remote uninstall              # tear down
    adb-connect daemon                        # run daemon in foreground (used by service unit)

## Why this exists

Android Studio's "Pair devices using Wi-Fi" is great — when you're on the phone's LAN. This tool makes the same flow work when you're not: the developer machine and the phone join a Tailscale mesh, a privileged Android companion app programmatically toggles wireless ADB, and the CLI's daemon brokers the connection over the mesh.

## Architecture and design

See [`docs/design.md`](docs/design.md) and the wire-protocol spec at [`docs/wire-protocol.md`](docs/wire-protocol.md).

## License

Apache-2.0 — see [`LICENSE`](LICENSE).
