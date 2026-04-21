# adb-connect

Connect `adb` to an Android phone from anywhere — same Wi-Fi, or across networks via [Tailscale](https://tailscale.com/). One command to pair, one command to connect, every time.

> Status: bootstrap. The product is under active development. Full design in [`docs/design.md`](docs/design.md).

## Quick tour

- **Same-LAN pairing** — `adb-connect pair` opens a browser tab with a QR code; scan it from the phone's Wireless Debugging settings and you're connected in seconds.
- **Remote over Tailscale** — `adb-connect remote setup` (once) then `adb-connect remote connect <phone>` gets your `adb` shell against a phone on a completely different network. Uses a tiny privileged companion app ("Premex ADB-gate") on the phone.

## Install (coming soon — v0.1.0)

```bash
# macOS / Linux
brew install premex-ab/tap/adb-connect

# or, anywhere
curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh
```

## Commands (design surface)

```
adb-connect pair                          # same-LAN QR flow
adb-connect remote setup                  # bootstrap Tailscale + daemon + companion app
adb-connect remote connect [nickname]     # connect to a remote phone
adb-connect remote status                 # list enrolled phones
adb-connect remote uninstall              # tear down
adb-connect daemon                        # run daemon in foreground (used by service unit)
```

## Why this exists

Android Studio's "Pair devices using Wi-Fi" is great — when you're on the phone's LAN. This tool makes the same flow work when you're not: the developer machine and the phone join a Tailscale mesh, a privileged Android companion app programmatically toggles wireless ADB, and the CLI's daemon brokers the connection over the mesh.

## License

Apache-2.0. See [`LICENSE`](LICENSE).
