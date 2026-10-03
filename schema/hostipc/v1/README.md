# Host IPC v1 — language-neutral schema

Contract version: `1.0` (Phase 0, CodeBridge V2). Frozen envelope (Provider Contracts §7).
This directory is the source of truth for the `codebridged` ↔ `CodeBridge.app` wire. Go and
Swift bindings are hand-written against these files; the schema is never generated from Go or
Swift structs.

## 1. Transport

- Unix domain socket in a `0700` per-user directory.
- Default path: `$HOME/Library/Application Support/CodeBridge/run/hostipc.sock`.
- Override: environment `CODEBRIDGE_HOSTIPC_SOCKET` or daemon flag `-host-socket`.
- `codebridged` listens; `CodeBridge.app` connects. The socket is unlinked and re-created on daemon start.
- No TCP listener exists for Host IPC in any configuration.

## 2. Framing

```
frame   := u32be header || payload[header & 0x7fffffff]
header  : bit 31 = 0  -> JSON control frame
          bit 31 = 1  -> binary attachment frame
          bits 0..30 = payload byte count (unsigned, big-endian)
```

- **JSON control frame** — payload is one complete UTF-8 JSON-RPC 2.0 message (object). No newline
  delimiter, no length inside the JSON.
- **Binary attachment frame** — payload is `u32be metaLen || meta JSON || raw bytes`. `meta` is an
  `attachment` object (`envelope.json#/$defs/attachment`). An attachment is always preceded by the
  control message that references `attachment_id`; the reference is what binds bytes to a message.
  Phase 0 sends no attachment frames (no Computer methods are implemented); the framing is defined
  now so Phase 1 does not renegotiate the wire.

Bounds (both peers MUST enforce them and fail closed):

| Frame | Maximum payload | Violation |
| --- | --- | --- |
| JSON control | 1 MiB (`1048576`) | error `-32020 frame_limit`, then close |
| binary attachment | 16 MiB (`16777216`) | error `-32020 frame_limit`, then close |

A payload length of `0`, a truncated frame, or a JSON control payload that is not a single JSON
object is a protocol error: `-32700 parse` (unreadable bytes) or `-32600 invalid_request`
(well-formed but not an object), then close.

## 3. Messages

Standard JSON-RPC 2.0. `jsonrpc` MUST be `"2.0"`.

- `request` — has `id` (string or number, echoed verbatim), `method`, optional `params` (object).
- `response` — has `id` and exactly one of `result` / `error`.
- `notification` — has `method`, no `id`. Notifications are never answered.

Bidirectional: the daemon both serves and issues requests/notifications to a connected app.

## 4. Handshake

The first message from a connecting app MUST be `host.hello`
(`hello.json#/$defs/helloRequest`). Any other message first is rejected with
`-32002 not_initialized`.

```
host.hello  params {protocol:{major:1,minor:0}, app_version, role:"app",
                    capabilities:["computer","approval","notify","runtime"], bundle_id?, pid?}
            result {accepted:true, protocol:{major:1,minor:0}, daemon_version:"0.1.0-phase0",
                    capabilities:[...], role:"app"}
```

Version rule:

- `major` MUST equal `1`. Any other value is `-32000 protocol_major_mismatch` and the connection is
  closed — a major mismatch disables the app's services (fail closed).
- Supported client minors are `{daemonMinor, daemonMinor-1}`. The daemon minor is `0` in Phase 0, so
  the only supported client minor is `0`. Outside that set: `-32001 protocol_minor_unsupported`, close.
- On acceptance the negotiated minor is `min(client, daemon)`.
- Unknown fields in any message are ignored. Unknown `capabilities` entries are ignored.

## 5. Authentication

Two independent checks, both fail closed:

1. **Peer UID.** `getpeereid`/`LOCAL_PEERCRED` on the accepted socket MUST equal the daemon's
   effective UID. Mismatch: `-32010 unauthorized_peer`, close.
2. **Live app code signature.** For `role:"app"`, the daemon reads the peer PID with
   `getsockopt(SOL_LOCAL, LOCAL_PEERPID)` on the live socket and validates the *running* process with
   the Security framework: `SecCodeCopyGuestWithAttributes({kSecGuestAttributePid})` +
   `SecRequirementCreateWithString("anchor apple generic and certificate leaf[subject.OU] = \"<TEAM>\" and identifier \"<BUNDLE>\"")`
   + `SecCodeCheckValidity`. Missing/failed: `-32010 unauthorized_peer`, close.

Never used for authentication: the self-reported `bundle_id`, the self-reported `pid`, the peer's
executable path, or a file-only `codesign` check. Those are hints for diagnostics only.

If the daemon has no configured signing identity (`-app-team-id` / `-app-bundle-id`), `role:"app"` is
rejected with `-32010 unauthorized_peer` and reason `signing_identity_not_configured`. There is no
debug bypass.

Roles other than `app` (`diagnostics`, `harness`) never require a code signature and never receive
`local_ui` or `computer` powers (§6).

## 6. Services, roles, direction and Phase 0 availability

The frozen ComputerProvider lives **inside CodeBridge.app**: the app is the provider and the daemon is
a broker. So, **over Host IPC**, `computer.*` (and the approval *presentation* calls) are requests the
**daemon sends to the app**, and the app answers. (The MCP ingress separately exposes that surface to
`remote_ai` callers; that is a different wire, see `docs/v2/architecture.md` §9.) The daemon never serves them, and a peer that sends one as a request
receives `-32601 unsupported` whatever its role.

Machine-readable table: `methods.json`. Directions:

| Direction | Meaning |
| --- | --- |
| `app_to_daemon` | the app/CLI sends it; the daemon answers |
| `daemon_to_app` | the daemon sends it; the app answers (provider side) |
| `daemon_to_app_notification` | one-way daemon → app |

| Method | Direction | Roles | Phase 0 |
| --- | --- | --- | --- |
| `host.hello` | app → daemon | app, diagnostics, harness | implemented |
| `host.health` | app → daemon | app, diagnostics, harness | implemented |
| `host.native_host_smoke` | app → daemon | app, diagnostics, harness | implemented (fixed check set, no caller argv) |
| `host.phase0_probe` | app → daemon | app, diagnostics | debug-gated (§7); otherwise unsupported |
| `host.prepare_restart` | app → daemon | app | `-32601 unsupported` (frozen, not faked) |
| `runtime.health` | app → daemon | app | served only when a runtime store is wired |
| `runtime.events` | app → daemon | app | served only when a runtime store is wired |
| `runtime.command` | app → daemon | app | `-32601 unsupported` (frozen, no fake no-op) |
| `approval.decision` | app → daemon | app | `-32601 unsupported` |
| `computer.describe` | **daemon → app** | app | `-32601 unsupported` |
| `computer.targets` | **daemon → app** | app | `-32601 unsupported` |
| `computer.open_session` | **daemon → app** | app | `-32601 unsupported` |
| `computer.observe` | **daemon → app** | app | `-32601 unsupported` |
| `computer.act` | **daemon → app** | app | `-32601 unsupported` |
| `computer.control` | **daemon → app** | app | `-32601 unsupported` |
| `computer.close_session` | **daemon → app** | app | `-32601 unsupported` |
| `approval.present` | **daemon → app** | app | `-32601 unsupported` |
| `approval.cancel` | **daemon → app** | app | `-32601 unsupported` |
| `notify.post` | **daemon → app** (notification) | app | implemented (daemon sends) |

Authorization order for an inbound request: unknown method or non-`app_to_daemon` direction →
`-32601 unsupported`; known `app_to_daemon` method with a role that may not call it →
`-32011 role_forbidden`; registered handler missing → `-32601 unsupported`.

A `diagnostics` or `harness` connection never receives `local_ui` or Computer powers: it can call
`host.hello`, `host.health` and `host.native_host_smoke` only, and gets `-32601` for every method the
daemon issues to the app.

## 7. `host.phase0_probe` (debug only)

Gated by `CODEBRIDGE_PHASE0_DEBUG=1`. Absent the gate the method is `-32601 unsupported`.
Allows `role:"app"` (live signature verified) or `role:"diagnostics"` (same UID), as in §6.
Params: `{probe:string, args?:[string], timeout_ms?}`; caller argv is ignored.
Harness names include `permissions`, `signing`, `native-host`, `harness`, `lock-state`, and
`input-monitor`. The daemon resolves `CODEBRIDGE_PHASE0_PROBE` (absolute executable) and
spawns that exact binary with fixed argv, without a shell or caller-selected executable.
The additional `daemon-permissions` and `daemon-files-folders` names call read-only macOS
native APIs **in the daemon PID**, without spawning a child. They require macOS/CGO and
the same explicit debug/probe configuration; other platforms return an explicit error.
Native ScreenCaptureKit calls have a 10s deadline each, folder reads 5s per root, and the
listen-only event tap a 3s observation window. These native deadlines are fixed, not
controlled by the harness `timeout_ms` override. An API timeout/error or an allocated tap
without observed delivery is not proof of a TCC denial. No pixels, filenames or input
content are reported. Result: `{exit_code, stdout, stderr, timed_out}`; `stdout` is the
native JSON report for these names, including actual PID, parent PID and signing identity.
`timed_out` describes a harness process deadline; inspect native API statuses separately.
There is no general argv exec method and no computer injection method in Host IPC v1.

## 8. Error codes

| Code | Name | Meaning |
| --- | --- | --- |
| `-32700` | `parse` | frame bytes are not valid JSON |
| `-32600` | `invalid_request` | JSON is not a JSON-RPC 2.0 object |
| `-32601` | `unsupported` | unknown or not-yet-implemented method |
| `-32602` | `invalid_params` | params do not match the schema |
| `-32603` | `internal` | daemon-side failure |
| `-32000` | `protocol_major_mismatch` | `major != 1`; connection closed |
| `-32001` | `protocol_minor_unsupported` | minor outside `{N, N-1}`; connection closed |
| `-32002` | `not_initialized` | non-hello first message |
| `-32010` | `unauthorized_peer` | peer UID or live code-signature failure; connection closed |
| `-32011` | `role_forbidden` | role not permitted for this method |
| `-32020` | `frame_limit` | frame payload outside bounds; connection closed |

## 9. Files

| File | Contents |
| --- | --- |
| `envelope.json` | JSON-RPC 2.0 envelope, error, attachment |
| `hello.json` | `host.hello` params/result, protocol, capabilities, roles |
| `host.json` | `host.*` params/results |
| `computer.json` | `computer.*` params/results (frozen contract, unimplemented in Phase 0) |
| `approval.json` | `approval.*` params/results |
| `notify.json` | `notify.post` params |
| `runtime.json` | `runtime.*` params/results |
| `methods.json` | method → request/result schema, direction, roles, Phase 0 availability |
