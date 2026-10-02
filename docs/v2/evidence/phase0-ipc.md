# Phase 0 — Host IPC + MCP ingress evidence (daemon side)

Owner: DaemonBaseline. Scope: `codebridged` Host IPC listener, MCP ingress (Streamable HTTP over a
Unix socket), authentication, store wiring, CLI. Baseline commit for this work: `8d1fcdf`.
V1 `Manager`/`Client`/`agentops` were not touched; this is an added process and packages only.

Everything below is **observed on this machine** (macOS 27.0 arm64, Go 1.26.5, CGO enabled) unless a
line is explicitly marked **[UNTESTED]** or **[BLOCKED]**.

## 1. Artifacts

| Path | Contents |
| --- | --- |
| `cmd/codebridged/main.go` | CLI: `run`, `smoke native-host`, `probe ingress`, `ctl …`, `tunnel doctor` |
| `internal/daemon/` | daemon lifecycle, MCP ingress + tools, Host IPC services, tunnel supervision, smoke, probe, store port + adapter |
| `internal/hostipc/` | Host IPC v1 Go binding: framing, JSON-RPC, handshake, auth, server, client |
| `schema/hostipc/v1/` | language-neutral frozen schema (README, envelope, hello, host, computer, approval, notify, runtime, methods) |
| `deploy/launchd/io.github.edynasty.codebridged.plist.example` | per-user LaunchAgent |
| `Makefile` | `build-daemon`, `run-codebridged` (CGO_ENABLED=1) |

Build:

```
make build-daemon          # CGO_ENABLED=1 go build -trimpath -o bin/codebridged ./cmd/codebridged
```

`codebridged` **requires cgo**: the `app` role is authorized by validating the *live* peer process
with the Security framework (`SecCodeCopyGuestWithAttributes` + `SecCodeRequirement` +
`SecCodeCheckValidity`). A `CGO_ENABLED=0` build fails closed: `role:"app"` is refused with
`-32010`. That is deliberate, not a gap.

## 2. Wire

Frozen contract: `schema/hostipc/v1/README.md` (a language-neutral schema, not generated from Go).
Header: `u32be`, bit 31 = 0 JSON control frame / 1 binary attachment frame, low 31 bits = payload
length; JSON ≤ 1 MiB, attachment ≤ 16 MiB, violations answered `-32020` then close. Default socket
`$HOME/Library/Application Support/CodeBridge/run/hostipc.sock`, overridable with
`-host-socket` / `CODEBRIDGE_HOSTIPC_SOCKET`. Directory `0700`, socket `0600`, no TCP listener in any
configuration.

Handshake and versions are as frozen with NativeSpike: `major != 1` → `-32000 protocol_major_mismatch`
(close); client minor must be in `{daemonMinor, daemonMinor-1}` → otherwise
`-32001 protocol_minor_unsupported` (close); negotiated minor is `min(client, daemon)`; any request
before `host.hello` → `-32002 not_initialized`. Verified by
`internal/hostipc/server_test.go` (`TestMajorMismatchClosesConnection`,
`TestMinorNegotiationNAndNMinus1`, `TestHelloRequiredBeforeOtherMethods`).

### 2.1 Authentication

| Check | Rule |
| --- | --- |
| Peer UID | `LOCAL_PEERCRED` at accept must equal the daemon UID, else close |
| App role | live PID re-read from the socket at `host.hello` (`LOCAL_PEERPID`); must be unchanged since accept; then `SecCodeCheckValidity` against `anchor apple generic and certificate leaf[subject.OU]="<team>" and identifier "<bundle>"` |
| CLI roles | `diagnostics` / `harness`: peer UID only, class `local_mcp`, never `local_ui` |
| MCP ingress | peer UID **and** (optional) per-launch bearer secret; bearer ⇒ `remote_ai`, absent ⇒ `local_mcp`, wrong ⇒ 401 |

Self-reported `pid`, `bundle_id`, `team_id`, and any executable path are **never** used for
authorization. There is no unsigned bypass on the daemon side.

Observed refusals (real Security-framework call, not a stub):

```
# no identity configured
$ bin/codebridged ctl health -host-socket $RUN/hostipc.sock -role app
signing_identity_not_configured (-32010): peer is not authorized: signing_identity_not_configured

# -app-team-id ABCDE12345 -app-bundle-id io.github.edynasty.codebridge
reason=app_signature_invalid error="hostipc: app_signature_invalid (pid 98176, OSStatus -67050):
SecCodeCheckValidity failed for requirement \"anchor apple generic and certificate leaf[subject.OU] = ...\""
```

`security find-identity -v -p codesigning` reports **0 valid identities** on this machine, so a
genuinely signed app peer has never been offered to the daemon. **[BLOCKED]**: the positive app-role
path (a signed `CodeBridge.app` accepted, `signature_verified: true`) cannot be exercised until a
Developer ID identity exists. The negative paths above are real.

### 2.2 Method availability and direction (Phase 0)

The ComputerProvider lives **inside CodeBridge.app**, so `computer.*` (and `approval.present` /
`approval.cancel`) are `daemon → app` requests: the app implements them and the daemon never serves
them. An inbound request for one of them — from any role — answers `-32601 unsupported`. That is the
authorization order: direction/existence is checked first, then role, then handler availability, so a
CLI role never learns more than "unsupported" about methods the daemon issues to the app.

For methods the daemon *does* serve (`host.*`, `approval.decision`, `runtime.*`), a role that may not
call them answers `-32011 role_forbidden`. `runtime.health`/`runtime.events` are registered at startup
only when a runtime store is wired (otherwise `unsupported`); `runtime.command` and
`host.prepare_restart` are frozen but deliberately unimplemented — they answer `-32601 unsupported`
rather than a fake result. `notify.post` is a daemon → app notification. Observed:

```
$ bin/codebridged ctl probe -host-socket $RUN/hostipc.sock -probe permissions
unsupported (-32601): unsupported: no handler for host.phase0_probe     # without the debug gate
```

```
$ bin/codebridged ctl call -host-socket "$RUN/hostipc.sock" -role harness -method <name>
computer.describe     unsupported (-32601): unsupported: no handler for computer.describe
computer.observe      unsupported (-32601): unsupported: no handler for computer.observe
computer.act          unsupported (-32601): unsupported: no handler for computer.act
approval.present      unsupported (-32601): unsupported: no handler for approval.present
notify.post           unsupported (-32601): unsupported: no handler for notify.post
runtime.health        role_forbidden (-32011): role "harness" may not call runtime.health
runtime.events        role_forbidden (-32011): role "harness" may not call runtime.events
runtime.command       role_forbidden (-32011): role "harness" may not call runtime.command
approval.decision     role_forbidden (-32011): role "harness" may not call approval.decision
host.prepare_restart  role_forbidden (-32011): role "harness" may not call host.prepare_restart
host.health           rc=0 (served; class=local_mcp)

$ bin/codebridged ctl call -host-socket "$RUN/hostipc.sock" -role app -method computer.describe
codebridged ctl: connect ...: signing_identity_not_configured (-32010): peer is not authorized
```

No Computer Use, no arbitrary exec, and no `notify.post` from a client. The frozen table with
direction/roles/availability per method is `schema/hostipc/v1/methods.json`.

### 2.3 MCP server lifetime (caller-class cache)

The MCP ingress builds **two** `*mcp.Server` instances once at daemon construction, keyed by caller
class: `remote_ai` (bearer-authenticated tunnel) and `local_mcp` (same-user, no bearer). The widget /
approval spike is registered on the `remote_ai` instance only, so a local connection can neither see
nor drive approval surfaces.

They are deliberately long-lived, not per request: each `tools/call` is its own HTTP request, so tool
state (approval tokens, widget sessions, later leases) must outlive a request. An ingress that built a
server per request silently lost that state — the bootstrap token of `phase0_approval` was minted on a
throwaway Probe and every follow-up decision was rejected. Regression:
`TestApprovalStateSurvivesAcrossHTTPRequests` (bootstrap over one request, decide over a second
session, reuse rejected) and `TestBrokenToolRegistrationFailsAtStartup`.

Building the servers eagerly also makes a broken tool registration fail **loudly at startup**. It used
to fail per request inside the HTTP handler, where `net/http` recovers the panic and closes the
connection: the client saw only `Post "http://localhost/mcp": EOF` and nothing was logged to the daemon
log (`http.Server.ErrorLog` writes to stderr, not to the slog file).

## 3. Commands the parent can run

```bash
make build-daemon
RUN=/tmp/cb-run; DATA=/tmp/cb-data; mkdir -p "$RUN"

# daemon (foreground; launchd uses the same argv)
bin/codebridged run -run-dir "$RUN" -data-dir "$DATA" -log /tmp/cb-daemon.log -tunnel=false &

# any Host IPC method, to see the frozen classification (debugging)
bin/codebridged ctl call -host-socket "$RUN/hostipc.sock" -role harness -method computer.describe
bin/codebridged ctl call -host-socket "$RUN/hostipc.sock" -role diagnostics -method runtime.health

# health + role separation (observed: class=local_mcp, tcp_listeners=0, mcp_socket_mode=-rw-------)
bin/codebridged ctl health   -host-socket "$RUN/hostipc.sock"
bin/codebridged ctl health   -host-socket "$RUN/hostipc.sock" -role app    # expect -32010
bin/codebridged ctl health   -host-socket "$RUN/hostipc.sock" -role harness

# integrated-authority native host smoke (runs inside the daemon process)
bin/codebridged ctl smoke    -host-socket "$RUN/hostipc.sock" -only shell,git,docker,ssh,kubectl,path

# MCP ingress auth matrix over the tunnel-client path
bin/codebridged probe ingress -socket "$RUN/mcp.sock" -token-file "$RUN/tunnel-token" -tool ping   # remote_ai
bin/codebridged probe ingress -socket "$RUN/mcp.sock" -no-token -tool ping                         # local_mcp

# opt-in widget/approval spike on the remote_ai instance (rebuild bin/ after tool-surface changes)
bin/codebridged run -run-dir "$RUN" -data-dir "$DATA" -tunnel=false -widget-spike &
bin/codebridged probe ingress -socket "$RUN/mcp.sock" -token-file "$RUN/tunnel-token" -tool ping
printf 'wrong-secret-0000' > /tmp/wrong-token && chmod 600 /tmp/wrong-token
bin/codebridged probe ingress -socket "$RUN/mcp.sock" -token-file /tmp/wrong-token -expect unauthorized
# observed: {"expected":"unauthorized","observed":"unauthorized"} rc=0
# (a missing token file exits 2 before the request; a present-but-wrong secret must be observed)
```

Debug-gated fixed probe subprocess (TCC attribution path):

```bash
CODEBRIDGE_PHASE0_DEBUG=1 CODEBRIDGE_PHASE0_PROBE=/abs/path/codebridge-probe \
  bin/codebridged run -run-dir "$RUN" -data-dir "$DATA" -tunnel=false &
bin/codebridged ctl probe -host-socket "$RUN/hostipc.sock" -probe permissions
# observed argv: ["<probe>", "permissions", "--json"] — caller-supplied args are ignored
```

## 4. Observed results

| Observation | Evidence |
| --- | --- |
| Sockets created `0700` dir / `0600` socket; only two Unix sockets, **no TCP/UDP listener** | `lsof -nP -p <daemon> -a -i` → empty; `-a -U` → exactly `hostipc.sock`, `mcp.sock` |
| Handshake as `diagnostics`/`harness` accepted without a signature, class `local_mcp` | log `hostipc handshake accepted role=diagnostics class=local_mcp signature_verified=false` |
| `app` role refused, reason recorded, no bypass | log `hostipc app role refused reason=signing_identity_not_configured bypass=none` |
| Per-launch bearer secret authenticates the ingress; absent ⇒ `local_mcp`; wrong ⇒ 401 | ingress probe matrix (§3) |
| Secret never logged; token file `0600` | `token leaked into log: False`; `oct(mode)=0o600` |
| Runtime store opened in the configured data dir, schema 1, journal empty | `runtime store opened path=/tmp/cb-data/runtime.db schema_version=1`; `stats{journal_pos:0}` |
| Graceful SIGTERM/`bootout`: sockets removed, exit 0, no stragglers | log `codebridged stopped`; run dir empty afterwards |
| `go test ./internal/hostipc/ ./internal/daemon/` | pass |
| Widget spike: ingress `initialize` + `tools/call` succeed with `-widget-spike` | `probe ingress` → `{"expected":"authorized","observed":"authorized", ...}` rc 0, daemon stderr clean |
| Cross-request approval state survives (bootstrap request → separate session decision → reuse rejected) | `TestApprovalStateSurvivesAcrossHTTPRequests` (was the per-request-server bug) |

### 4.1 launchd lifecycle (observed, real binary)

A throwaway per-user LaunchAgent was bootstrapped and booted out (label
`io.github.edynasty.codebridged.phase0test`, plist removed afterwards):

```
$ launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/<label>.plist
sockets appeared under launchd: True ['hostipc.sock','mcp.sock','tunnel-token']
launchctl print: state = running | program = …/bin/codebridged | pid = 98969 | last exit code = (never exited)
ps: 98969     1 …/bin/codebridged run -run-dir /tmp/cb-launchd/run -data-dir /tmp/cb-launchd/data -tunnel=false -log …
health via launchd-managed daemon: store_wired=True tcp_listeners=0
$ launchctl bootout gui/$(id -u)/<label>
after bootout: (none)
log: INFO msg="codebridged shutdown requested" / INFO msg="codebridged stopped"
```

`ppid = 1` confirms launchd owns the process; `KeepAlive{SuccessfulExit:false}` is the restart policy.
The daemon boots, serves both sockets and the store, and shuts down gracefully on `bootout` (SIGTERM)
with no stragglers.

Store integration is real, not a seam: `internal/daemon/store_runtime.go` adapts
`internal/runtime.*Store` (Open/Ping/Stats/EventsAfter/EventsForStream/JournalHead) to the daemon's
narrow `Store` port, and `cmd/codebridged` opens it at startup (`-runtime-db`, default
`<data-dir>/runtime.db`). Phase 0 creates no runs — the daemon only reads.

## 5. [UNTESTED] / gaps

- Positive `app`-role acceptance with a real signed `CodeBridge.app` (no signing identity exists).
- The daemon → app provider calls themselves: `CodeBridge.app` is **client-only** in Phase 0
  (it dials hostipc.sock and calls `host.*`; it registers no handler for `computer.*`,
  `approval.present`/`approval.cancel` or `notify.post`). The daemon side of that direction is
  therefore classified and tested only against a Go client, never against a live provider.
- Real Go/Swift client handshake and native live-signature rejection were subsequently exercised by the parent (see §6). Positive genuine signed-app acceptance is still blocked.
- Real ChatGPT connector round-trip through the tunnel (see `phase0-tunnel.md`).
- `-32020`/`-32700`/`-32600` paths are unit-tested, not observed against the Swift client.
- Socket paths longer than 100 bytes are rejected with a clear error (macOS limit); the default
  Application Support path is well under it.

## 6. Parent real Go/Swift and protocol smoke

Actual Swift diagnostics client connected to the real launchd Go daemon over UDS: protocol 1.0, same UID/audit-token peer identity, explicit diagnostics-only ad-hoc opt-in. The `app` role never receives that opt-in: live Apple-anchored verification refused the daemon with `SecCodeCheckValidity -67050` even with the opt-in environment set. Missing daemon identity and missing App identity also fail closed. No genuinely signed positive result is claimed.

Actual framed requests returned: major 2 → `-32000`, unsupported minor 1 → `-32001`, health before hello → `-32002`, inbound `computer.act` (wrong direction) → `-32601`, diagnostics `runtime.health` → `-32011`, unknown method → `-32601` with the connection still usable. A self-reported PID 999999 did not override the observed socket peer PID. Native handshake, socket/parent permissions and real health passed. [Sanitized parent results](phase0-parent-smoke.json).

Conclusion: Task 11 **PASS** for the requested minimal protocol and negative security paths. Version 1.0 has no predecessor minor; the N/N−1 rule is defined and tested at higher minors, not falsely demonstrated by accepting future 1.1. Provider methods remain unavailable; the genuinely signed production peer is a separate blocked identity/TCC gate. No Swift/Go/upstream struct serves as the wire specification.

