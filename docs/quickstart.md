# adb-connect — 60-second quickstart

## Install

    brew install premex-ab/tap/adb-connect
    # or
    curl -fsSL https://premex-ab.github.io/adb-connect/install.sh | sh

## Use case 1 — phone on the same Wi-Fi

On your Android phone: **Settings → Developer options → Wireless debugging** — leave the panel open.

On your laptop:

    adb-connect pair

A browser tab opens with a QR. Scan it from the **Pair device with QR code** option on the phone. Within seconds, the phone appears in `adb devices`.

## Use case 2 — phone on a different network (over Tailscale)

One-time setup (both the laptop and phone must already be on the same tailnet):

    adb-connect remote setup

Follow the prompts: paste a Tailscale auth key, pick a nickname for the phone, scan the enrollment QR from the Premex ADB-gate app on the phone, flip the app's toggle to ON.

Everyday use:

    adb-connect remote connect              # auto-picks if one phone
    adb-connect remote connect my-pixel     # by nickname
    adb-connect remote status               # list enrolled phones

The phone is reachable via its Tailscale IP — no same-LAN requirement.

## Uninstall

    adb-connect remote uninstall           # keeps config
    adb-connect remote uninstall --wipe-config   # full teardown
