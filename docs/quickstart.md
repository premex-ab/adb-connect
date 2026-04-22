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

## Everyday use

    adb-connect pair

A QR code opens in your browser. On the phone: **Wireless debugging → Pair device with QR code**, then scan. Within seconds the phone appears in `adb devices`.

The Premex ADB-gate toggle must be ON for wireless ADB to be active. The app shows the current ADB endpoint (IP:port) when it is.

## Commands

    adb-connect pair            # QR pair + connect (same-LAN)
    adb-connect install-app     # sideload the companion app (once per phone / release)
    adb-connect version         # print CLI version
