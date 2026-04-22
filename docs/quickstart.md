# adb-connect — 60-second quickstart

## Install

    brew install premex-ab/tap/adb-connect
    # or
    curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh

## Setup (one time per phone)

1. Plug the phone in via USB, or enable **Wireless debugging** once from Developer options and run `adb-connect pair`.

2. Sideload the signed Premex ADB-gate companion app:

       adb-connect install-app

   This downloads the signed APK from GitHub Releases, verifies its SHA-256, installs it, and grants the `WRITE_SECURE_SETTINGS` permission.

3. Open **Premex ADB-gate** on the phone and flip the master toggle **ON**.

4. Install the background watcher so future reconnects are automatic (recommended):

       adb-connect service install

   This registers `adb-connect watch` as a launchd user agent (macOS) or systemd-user unit (Linux). From now on, toggling **Premex ADB-gate ON** on the phone causes the laptop to auto-`adb connect` within a few seconds — no manual command needed.

## Everyday use (after service install)

1. Toggle **Premex ADB-gate ON** on the phone.
2. Within ~5 seconds the phone appears in `adb devices` automatically.

That's it. No QR scan, no manual `adb connect`.

## Everyday use (without service install)

    adb-connect pair

A QR code opens in your browser. On the phone: **Wireless debugging → Pair device with QR code**, then scan. Within seconds the phone appears in `adb devices`.

Or run the watcher in the foreground for one session:

    adb-connect watch

## Commands

    adb-connect pair                # QR pair + connect (same-LAN)
    adb-connect install-app         # sideload the companion app (once per phone / release)
    adb-connect watch               # foreground mDNS watch loop (auto-connect paired phones)
    adb-connect service install     # install watch as a login service (recommended one-time setup)
    adb-connect service uninstall   # remove the login service
    adb-connect version             # print CLI version
