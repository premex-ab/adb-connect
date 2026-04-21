# adb-connect wire protocol

The Premex ADB-gate phone app talks to the adb-connect daemon over a WebSocket on the Tailscale mesh. All frames are JSON objects with an `op` field identifying the message type. Additional fields depend on `op`.

## Transport

- `ws://<server-tailscale-host>:<ws-port>` — the daemon binds on its tailnet interface (never LAN or public internet).
- One connection per phone. On drop, the phone reconnects with exponential backoff (base 500 ms, cap 60 s).

## Authentication

- Single 256-bit pre-shared key (PSK), base64-encoded. Delivered to the phone via an enrollment QR (displayed during `adb-connect remote setup`).
- The phone's first frame must be `hello` with the PSK. Daemon validates with constant-time compare.
- Wrong PSK → `error(auth_failed)` → daemon closes the connection.
- Unknown nickname → `error(unknown_phone)` → daemon closes.

## Frames

### hello (phone → server, first frame)

Sent immediately after the WebSocket handshake. Must be the very first frame; any other op before `hello` causes the server to respond with `error(auth_required)` and close.

```json
{
  "op": "hello",
  "nickname": "my-pixel",
  "psk": "<base64-encoded 256-bit key>",
  "app_version": "0.1.0"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `op` | string | yes | Always `"hello"` |
| `nickname` | string | yes | Human-readable phone identifier, set during enrollment |
| `psk` | string | yes | Base64-encoded pre-shared key |
| `app_version` | string | no | Semver string of the companion app build |

### ack (server → phone)

Sent by the server after a successful `hello` validation. Carries no additional fields.

```json
{
  "op": "ack"
}
```

After receiving `ack` the phone is considered enrolled for this session. The server records `last_ws_seen` for `status` reporting.

### toggle_state (phone → server)

Sent whenever the user flips the toggle in the ADB-gate app UI. Tells the daemon whether wireless ADB is currently enabled on the phone.

```json
{
  "op": "toggle_state",
  "on": true
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `op` | string | yes | Always `"toggle_state"` |
| `on` | bool | yes | `true` when wireless ADB is active on the phone |

The `on` field is explicitly required — the server rejects a `toggle_state` that omits it with `error(bad_frame)`.

### prep_connect (server → phone)

Sent by the server when the user runs `adb-connect remote connect` on the laptop. Instructs the phone to enable ADB-over-Wi-Fi and discover its address/port via mDNS.

```json
{
  "op": "prep_connect",
  "request_pair": false
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `op` | string | yes | Always `"prep_connect"` |
| `request_pair` | bool | yes | `true` on the first connect when the phone is not yet paired; `false` for subsequent connects that only need `adb connect` |

When `request_pair` is `true`, the phone is expected to run mDNS discovery for `_adb-tls-pairing._tcp` and include the `pair_code` in its `connect_ready` response. When `false`, it discovers `_adb-tls-connect._tcp` instead and omits `pair_code`.

### connect_ready (phone → server)

Sent by the phone in response to `prep_connect` once it has discovered its Tailscale IP and the mDNS-advertised ADB port.

```json
{
  "op": "connect_ready",
  "ip": "100.64.0.42",
  "port": 37049,
  "pair_code": "123456"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `op` | string | yes | Always `"connect_ready"` |
| `ip` | string | yes | Tailscale IPv4 address of the phone |
| `port` | int | yes | Port advertised by the ADB mDNS record (non-zero) |
| `pair_code` | string | no | Six-digit pairing code; present only when the server sent `prep_connect` with `request_pair: true` |

The server rejects a `connect_ready` that omits `ip` or `port` with `error(bad_frame)`.

### error (either direction)

Sent by either side to signal a terminal failure. The sender may close the connection immediately after.

```json
{
  "op": "error",
  "code": "auth_failed",
  "message": "PSK mismatch"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `op` | string | yes | Always `"error"` |
| `code` | string | yes | Machine-readable error identifier (see table below) |
| `message` | string | yes | Human-readable detail, suitable for display to the developer |

## State machine

1. Phone: open WS → send `hello`.
2. Server: validate PSK + nickname → send `ack`, record `last_ws_seen`.
3. (Optional) Phone sends `toggle_state` whenever the user toggles on the app UI.
4. When laptop runs `adb-connect remote connect`:
   - Server sends `prep_connect {request_pair}`.
   - Phone enables ADB-over-Wi-Fi (via `WRITE_SECURE_SETTINGS`), runs mDNS discovery for `_adb-tls-pairing._tcp` (if pairing) or `_adb-tls-connect._tcp` (if already paired), reports its Tailscale IPv4 + mDNS-discovered port.
   - Phone sends `connect_ready {ip, port, pair_code?}`.
   - Server runs `adb pair ip:port pair_code` (first time only) then `adb connect ip:port`.
5. If anything fails on the phone side, it sends `error` with a machine-readable `code`. The server relays this to the CLI caller immediately.

## Error codes

| code | origin | meaning |
|---|---|---|
| `auth_required` | server | first frame wasn't hello |
| `auth_failed` | server | PSK mismatch |
| `unknown_phone` | server | nickname not enrolled |
| `bad_frame` | either | unknown op / missing required field |
| `phone_offline` | server | no WS connection for the requested nickname |
| `connect_timeout` | server | no connect_ready within 20 s of prep_connect |
| `discover_failed` | phone | mDNS discovery timed out (user hasn't opened the pair dialog) |
| `pair_failed` | server | `adb pair` exited non-zero |
| `connect_failed` | server | `adb connect` exited non-zero |

## Versioning

Protocol is v1. New frame types or required fields bump the minor version in the `hello.app_version` field; the server warns but does not hard-reject minor mismatches. Major version bumps (breaking changes) reject incompatible clients with `error(bad_frame)`.
