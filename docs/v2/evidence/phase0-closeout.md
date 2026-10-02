# Phase 0 closeout — 2026-10-02

**PHASE_0_BLOCKED — Phase 1 NOT READY.** Architecture frozen at `8d1fcdf`; no frozen topology or trust decision changed. [中文版](phase0-closeout.zh-CN.md).

## Phase 0 Summary

The minimal Go daemon, native Swift app/probe, UDS MCP ingress, Host IPC contracts, durable Store and explicit capability definitions are implemented and exercised. V1 Manager/Client/RunManager/RunStore/adapters remain unchanged and passed the repository-wide Go gate. This is not a complete Computer Use engine, scheduler, approval presenter or media implementation.

**F2 safety acceptance failed to close:** an actual launchd daemon child captured a real frame; existing TCC logs attribute the ScreenCapture request to the daemon. Computer permission/input experiments stopped. Genuine signed App/daemon/harness isolation, signed protected-root grant retention and real ChatGPT/Tunnel approval remain missing prerequisites, not deferred Phase 1 work.

## Checklist

`FAIL` below means the required acceptance evidence is missing or the safety gate is unresolved; it does not imply that the frozen architecture is proven impossible. Only Task 10 is DEFERRED, as the roadmap explicitly makes it a Phase 2 gate.

| User task | Result | Observed evidence / missing acceptance |
| --- | --- | --- |
| 1 — upstream audit | **PASS** | `Fanch-hui/codex-bridge`, `win`, tag `v1.3.4`, exact SHA `7844bb608a9a4e96ed09c084589b7825db77aa3e`; Apache-2.0 LICENSE/NOTICE and full 12-row capability matrix. Unchanged SwiftPM core build and actual service/SQLite smoke passed; upstream has no test targets; full `.app` Xcode build stalled and is **not verified**. Audit PASS does not certify that app build. No upstream code/types imported. [Audit](phase0-upstream.md). |
| 2 — Secure MCP Tunnel | **FAIL / BLOCKED** | Real UDS MCP initialize/tools/ping, bearer classes and real tunnel-client supervision passed locally. Control-plane tunnel ID/API key absent; actual ChatGPT/Platform browser required login. No genuine ChatGPT→Tunnel round trip, live tunnel recovery or disconnect isolation proof. [Tunnel](phase0-tunnel.md). |
| 3 — daemon lifecycle | **FAIL / BLOCKED** | Manual per-user launchd fixture: ppid 1, crash restart 62447→72321, Store reopen, bearer rotation, graceful bootout. Genuine signed `SMAppService` registration and App-quit survival on that registered path not exercised: zero valid signing identities/Team ID. [Lifecycle](phase0-launchagent.md). |
| 4 — native host | **PASS** | Actual launchd daemon ran shell/git/Docker Desktop **Server**/SSH/kubectl; all exit 0; argv, stdout/stderr, PATH/HOME/USER, cwd `/` recorded. No Manager or container execution. [Native host](phase0-native-host.md), [machine results](phase0-parent-smoke.json). |
| 5 — signed TCC isolation | **FAIL — ARCHITECTURE_BLOCKER F2** | Real child PID 77878 of daemon 72321 captured 1800×1169 pixels in memory. Existing TCC request `77878.1` names daemon as responsible and authorization subject, authValue 2. Genuine signed App-grant inheritance causality **not established**; original grant source unknown. Experiments stopped. [TCC](phase0-tcc.md), [exact attribution](phase0-tcc-attribution.txt). |
| 6 — protected roots | **FAIL / BLOCKED** | Desktop/Documents/Downloads measured blocked at first launchd deadlines, later readable; ordinary `/tmp` readable. No genuinely signed grant comparison across rebuild/update. Neither `SUPPORTED` nor `UNSUPPORTED_IN_V1` can honestly be selected; this required release decision remains blocked, not “maybe supported.” [TCC §7](phase0-tcc.md#7-parent-integration-f2-stop-condition). |
| 7 — lock/saver/sleep/wake/FUS | **FAIL / BLOCKED** | Actual locked startup snapshot suspended model, controller none, epochs/geometry invalidated, one admitted model action discarded. No transitions observed during measured windows; no OS capture/injection/queue-replay transition evidence or second-user GUI session. Earlier rejected queue seed metric corrected; model tests are not OS proof. [Lock evidence](phase0-lock-state.md). |
| 8 — input monitoring | **FAIL — positive feasibility unverified** | Listen-only tap measured; Secure Input active forced unavailable. No reliable physical/third-party/own-injected discrimination or preemption evidence. `computer.input` remains unavailable; no injection performed. F2 stops further dependent experiments. [Input evidence](phase0-input-monitor.md). |
| 9 — approval-only widget | **FAIL / BLOCKED** | Real UDS separate-request approval decision, token only in `_meta`, once/session, replay/missing/wrong-widget/project/always refusals, no grant creation; local HTML exercised. No authenticated real ChatGPT widget, so model hiding/UI-only host enforcement and host widgetSessionId cases unproven. [Widget evidence](phase0-widget-approval.md). |
| 10 — WebRTC feasibility | **DEFERRED — allowed Phase 2 gate only** | Actual local HTML RTC offer/answer/data-channel smoke passed; no real ChatGPT Web/Desktop/Mobile/CSP/STUN/TURN proof. Does not waive Task 9. Frozen stateless rendezvous fallback only if actual widget infeasibility is demonstrated. [Media evidence](phase0-webrtc.md). |
| 11 — Host IPC v1 | **PASS — minimum protocol** | Language-neutral schema, framing, hello/version/capabilities/health; real Go↔Swift UDS handshake. 0700 parent/0600 sockets, peer UID/audit identity, live Apple-anchored signature refusals, version/direction/role negatives. Positive genuine signed-App peer is separately blocked; Computer/approval/notify provider methods not claimed implemented. [IPC](phase0-ipc.md). |
| 12 — Store schema v1 | **PASS** | Required entities and schema_version ledger, Application Support default, WAL/FULL/FK, backed-up forward migration, newer-schema refusal, atomic state+event and seq/pos continuation; transaction/reopen smoke and migration regressions passed. No V1 import. [Store](phase0-store.md). |
| explicit capabilities | **PASS — definitions only** | Required Computer and Agent flags, including false values; five Phase 1 tool schemas and policy/visibility requirements. JSON Schema validation and missing-flag negatives passed. No OS/provider-type inference; no available Computer tool asserted. [Capabilities](phase0-capabilities.md). |

## Architecture Blockers

### F2 — Computer privilege isolation not established

Assumption: daemon/harnesses must not gain Computer TCC authority through App/parent responsibility. Environment: macOS 27.0 (26A428), arm64, UID 501, ad-hoc binaries, no genuine Team ID. Steps: real launchd daemon ran its fixed native permission probe; after the unexpected successful capture, only **read-only existing unified logs** were inspected.

Observed: ScreenCapture initially denied `-3801`, later capture succeeded (1800×1169; SHA-256 only persisted). Request `77878.1` attributes responsible PID **72321**, identifier **a.out**, and subject **bin/codebridged**; authValue **2**, authReason **4**, no error. Pixels, keystroke contents and protected-root names were not persisted. Raw GUI launchd environment and bearer/token values are excluded from repository evidence.

Conclusion: **ARCHITECTURE_BLOCKER F2**. This is actual daemon-attributed capture, **not** proof of causal inheritance of a genuinely signed App grant and **not** proof that the frozen two-process topology is impossible. The original grant source remains unknown. The signed production boundary cannot be approved on ad-hoc denial/success measurements.

Required minimum architecture amendment/containment: **use the already frozen F2 contingency** — independently signed daemon identity and disclaimed-responsibility harness spawning, followed by a controlled genuinely signed App/daemon/harness grant comparison before any Computer input ships. No redesign, automatic grant/reset, weakened signature check or speculative fallback was implemented. The owned LaunchAgent was booted out; daemon logged shutdown requested/stopped. All dependent Computer permission/input experiments remain stopped.

Other blocking prerequisites: genuine Developer ID identity + Team ID; operator-controlled lock/saver/sleep/wake and second-user session; authenticated ChatGPT developer mode plus workspace-associated tunnel ID/runtime API key. Browser login checks, environment/config inspection and signing-identity checks did not find these. Real tests must close these gates; local substitutes do not.

## Tests and real smoke

| Executed command / scenario | Result |
| --- | --- |
| `make fmt-check`; `gofmt -l schema/store/v1` | PASS; no unformatted files |
| `go test ./...` | PASS; 17 test packages, 8 packages with no tests; V1 included |
| `go vet ./...`; `go build ./...`; `make build-daemon` | PASS; daemon cgo live Security-framework verification enabled |
| `go run ./cmd/store-smoke` | PASS; illegal terminal transition rejected; contiguous stream, controller event, reopen seq 6 / pos 9 |
| `swift build -c release --package-path desktop/macos` | PASS |
| `swift test --package-path desktop/macos` | PASS; **33 tests**, zero failures |
| `sh -n` and `bash -n` for all three `desktop/macos/Building/*.sh` scripts | PASS |
| `build-app.sh --unsigned-dev --daemon bin/codebridged --embed-phase0-env` | PASS development bundle / plist assembly; explicitly **not** genuine signing/TCC/SMAppService evidence |
| `git diff --check` | PASS |
| LaunchAgent actual daemon + Host IPC + native host + SIGKILL restart + bootout | PASS local lifecycle/transport/native-host subset; signed App registration still blocked |
| MCP ingress Go SDK and direct UDS HTTP; separate-request approval | PASS local transport/token-state subset; actual Tunnel/ChatGPT blocked |
| Real tunnel-client supervision | Flags/config validation, bounded restarts and clean shutdown observed; control-plane connection **not achieved** |
| Real permission probe / existing TCC attribution log | **F2 blocker observed**; signed isolation not verified |
| Lock/input probe | Locked snapshot and Secure Input negative path observed; real transition/preemption not verified |
| Actual local widget browser / RTC data channel | PASS local surface; real ChatGPT widget acceptance not achieved |
| Upstream unchanged core build / service smoke | PASS; upstream `swift test` exited 1 (no targets); full app Xcode build unverified |

No source-text/wording/wiring-only Swift tests were retained as safety evidence. Regression tests cover token consumption, cross-request state, authorization/versions, Store constraints/rollback/migrations and suspension/frame invariants. No complete injector or real media transport was added.

## Commits

**None created. No push.** Independent milestones remain as working-tree changes; this blocked evidence is not packaged as a claim of Phase 0 completion. No reset/clean or V1 deletion.


Final evidence hygiene: all **124 manifest files exist**, all JSON artifacts parse, and **148 relative Markdown links resolve**; Markdown trailing-whitespace scan passed. Owned stopped smoke fixtures were removed only after sanitized evidence was preserved and endpoints/token files were absent; final `git diff --check` passed.
## Remaining P2 — nonblocking only

- Actual host `openai/widgetSessionId` availability is unproven; without a server-received binding the spike relies on token possession. Frozen gate explicitly allows this residual risk; scopes are not widened.
- Real Widget Web/Desktop/Mobile media, CSP and restrictive-network STUN/TURN evidence is deferred solely to the **Phase 2 gate**.

Signed TCC, protected-root outcome, actual lock/FUS safety and real Phase 1 approval/ingress are **blockers**, not P2 leftovers. Input feasibility also remains unverified and input unavailable; the original freeze P2 label does not authorize shipping it.

## Final

```text
PHASE_0_BLOCKED
Phase 1 readiness: NOT READY
Blocking assumption: F2 Computer TCC isolation is not established;
                    signed native and real ChatGPT/Tunnel gates remain open.
Required architecture amendment: frozen F2 signing/responsibility contingency,
                                 followed by genuine signed isolation evidence.
```

## Files Changed

The exhaustive working-tree manifest follows; generated binaries, SwiftPM caches, development bundle and owned temporary fixtures are excluded.
```text
Makefile
cmd/codebridged/main.go
cmd/store-smoke/main.go
deploy/launchd/io.github.edynasty.codebridged.plist.example
desktop/macos/.gitignore
desktop/macos/Building/Info.plist.in
desktop/macos/Building/build-app.sh
desktop/macos/Building/check-signing-identity.sh
desktop/macos/Building/com.codebridge.daemon.plist.in
desktop/macos/Building/smoke-native.sh
desktop/macos/Package.swift
desktop/macos/Sources/CodeBridgeApp/AppDelegate.swift
desktop/macos/Sources/CodeBridgeApp/DaemonClient.swift
desktop/macos/Sources/CodeBridgeApp/LaunchAgentController.swift
desktop/macos/Sources/CodeBridgeApp/MenuBarController.swift
desktop/macos/Sources/CodeBridgeApp/ProbeChildRunner.swift
desktop/macos/Sources/CodeBridgeApp/main.swift
desktop/macos/Sources/CodeBridgeComputerSpike/InputClassifier.swift
desktop/macos/Sources/CodeBridgeComputerSpike/ProbeReport.swift
desktop/macos/Sources/CodeBridgeComputerSpike/SuspensionModel.swift
desktop/macos/Sources/CodeBridgeIPC/Framing.swift
desktop/macos/Sources/CodeBridgeIPC/HostIPCClient.swift
desktop/macos/Sources/CodeBridgeIPC/HostIPCPaths.swift
desktop/macos/Sources/CodeBridgeIPC/JSONRPC.swift
desktop/macos/Sources/CodeBridgeIPC/JSONValue.swift
desktop/macos/Sources/CodeBridgeIPC/PeerIdentity.swift
desktop/macos/Sources/CodeBridgeIPC/PeerTrust.swift
desktop/macos/Sources/CodeBridgeIPC/ProcessIdentity.swift
desktop/macos/Sources/CodeBridgeIPC/ProtocolVersion.swift
desktop/macos/Sources/CodeBridgeIPC/ShellCommand.swift
desktop/macos/Sources/CodeBridgeIPC/UnixSocket.swift
desktop/macos/Sources/codebridge-probe/AccessibilityProbe.swift
desktop/macos/Sources/codebridge-probe/AsyncBridge.swift
desktop/macos/Sources/codebridge-probe/FilesFoldersProbe.swift
desktop/macos/Sources/codebridge-probe/HostEnvironment.swift
desktop/macos/Sources/codebridge-probe/HostIPCHandshakeProbe.swift
desktop/macos/Sources/codebridge-probe/HostToolsProbe.swift
desktop/macos/Sources/codebridge-probe/InputMonitorProbe.swift
desktop/macos/Sources/codebridge-probe/LockStateProbe.swift
desktop/macos/Sources/codebridge-probe/ProbeOptions.swift
desktop/macos/Sources/codebridge-probe/ProbeRunner.swift
desktop/macos/Sources/codebridge-probe/ScreenCaptureProbe.swift
desktop/macos/Sources/codebridge-probe/main.swift
desktop/macos/Tests/CodeBridgeComputerSpikeTests/SuspensionSpikeTests.swift
desktop/macos/Tests/CodeBridgeIPCTests/HostIPCWireTests.swift
docs/v2/README.md
docs/v2/README.zh-CN.md
docs/v2/architecture.md
docs/v2/architecture.zh-CN.md
docs/v2/evidence/phase0-capabilities.md
docs/v2/evidence/phase0-closeout.md
docs/v2/evidence/phase0-closeout.zh-CN.md
docs/v2/evidence/phase0-input-monitor.md
docs/v2/evidence/phase0-ipc.md
docs/v2/evidence/phase0-launchagent.md
docs/v2/evidence/phase0-lock-state.md
docs/v2/evidence/phase0-native-host.md
docs/v2/evidence/phase0-parent-smoke.json
docs/v2/evidence/phase0-store.md
docs/v2/evidence/phase0-tcc-attribution.txt
docs/v2/evidence/phase0-tcc.md
docs/v2/evidence/phase0-tunnel.md
docs/v2/evidence/phase0-upstream.md
docs/v2/evidence/phase0-webrtc.md
docs/v2/evidence/phase0-widget-approval.md
docs/v2/migration.md
docs/v2/migration.zh-CN.md
docs/v2/roadmap.md
docs/v2/roadmap.zh-CN.md
internal/approvalspike/probe.go
internal/approvalspike/probe_test.go
internal/approvalspike/widget.html
internal/daemon/config.go
internal/daemon/daemon.go
internal/daemon/daemon_test.go
internal/daemon/exec.go
internal/daemon/hostipcapi.go
internal/daemon/ingress.go
internal/daemon/mcpclient.go
internal/daemon/mcpserver.go
internal/daemon/probe.go
internal/daemon/smoke.go
internal/daemon/store.go
internal/daemon/store_runtime.go
internal/daemon/tunnel.go
internal/hostipc/client.go
internal/hostipc/errors.go
internal/hostipc/frame.go
internal/hostipc/frame_test.go
internal/hostipc/hello.go
internal/hostipc/jsonrpc.go
internal/hostipc/methods.go
internal/hostipc/peer_darwin.go
internal/hostipc/peer_other.go
internal/hostipc/server.go
internal/hostipc/server_test.go
internal/hostipc/sigcheck_darwin.go
internal/hostipc/sigcheck_other.go
internal/runtime/entities.go
internal/runtime/entities_ops.go
internal/runtime/events.go
internal/runtime/events_test.go
internal/runtime/migrate.go
internal/runtime/migrate_test.go
internal/runtime/paths.go
internal/runtime/policy_ops.go
internal/runtime/runs_ops.go
internal/runtime/store.go
internal/runtime/store_test.go
internal/runtime/transitions.go
internal/runtime/tx.go
schema/capabilities/v1/provider.schema.json
schema/capabilities/v1/tools.json
schema/hostipc/v1/README.md
schema/hostipc/v1/approval.json
schema/hostipc/v1/computer.json
schema/hostipc/v1/envelope.json
schema/hostipc/v1/hello.json
schema/hostipc/v1/host.json
schema/hostipc/v1/methods.json
schema/hostipc/v1/notify.json
schema/hostipc/v1/runtime.json
schema/store/v1/schema.go
schema/store/v1/schema.sql
```

