# Phase 0B — Blocker Closure / Signed Native Acceptance

Date: 2026-10-02. Host: macOS 27.0 (26A428), arm64.

**PHASE0_BLOCKED**

- F2: **NOT CLOSED**.
- Phase 1: **NOT READY**. No Phase 1 implementation or Computer input enabled.
- **SIGNED_TCC_BLOCKED_EXTERNAL**: current keychain reports zero valid signing identities.
- **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**: no usable Tunnel credentials/authenticated ChatGPT acceptance surface accessible.
- **CONTINGENCY_NOT_PUBLICLY_SUPPORTED**: responsibility disclaimer has no established supported public implementation; no private API/SPI added.

[中文](phase0b-closeout.zh-CN.md). Historical [Phase 0 closeout](phase0-closeout.md) and exact [TCC attribution](phase0-tcc-attribution.txt) are preserved, not rewritten.

## Blocking items and evidence

| Blocking item | Observed evidence | Classification | Minimum next action |
| --- | --- | --- | --- |
| Genuine signing identity | actual security command: 0 valid identities; builder refuses before replacing output | external certificate/private-key prerequisite; no signed experiment failed | install Apple Development certificate with private key (preferred), or Developer ID Application; confirm a real valid identity/Team |
| F2 signed isolation | no signed A–E matrix, genuine SMAppService lifecycle or grant-persistence repeat; historical daemon-authorized capture remains | unresolved security evidence gap, not a new signed topology-failure result | genuine tree → SMAppService lifecycle → controlled App-only Computer grants → actual A–E attribution/restart/rebuild repeats |
| Direct C API evidence | current daemon TCC diagnostics execute in harness child, not daemon PID | native acceptance instrumentation/evidence gap, not a DENIED result | before claiming C, add/exercise a read-only native probe executing in the actual signed daemon PID; never relabel D as C |
| Protected-root decision | only ordinary owned-root boundaries smoked; no signed protected-root grant persistence | external signing prerequisite; neither SUPPORTED nor UNSUPPORTED_IN_V1 established | independent Files & Folders attribution/grant/daemon-restart/same-identity update acceptance |
| Real lock/input | deliberately not run because F2 remains open | downstream security gate | close F2 first, then manual OS transitions and physical/own/third-party/unknown/Secure Input delivery matrix |
| Actual ChatGPT/Tunnel/widget | managed login surfaces; no configured Tunnel keys; actual-browser relay extension disconnected | external account/credentials/browser-access prerequisite, not transport failure | accessible developer-mode conversation + real Tunnel configuration; real pending-operation UI approval and original-operation resumption |

Certificate absence does not establish architecture failure and is not a reason to amend F2. The historical daemon/harness capture remains an unresolved F2 finding; its original grant history and signed App causality are unknown. If future history is contaminated, label **EVIDENCE_CONTAMINATED**, do not force PASS or reset the whole user. No TCC reset/database write/automatic grant was performed.

## Per-item result

| Requested item | Result | Evidence |
| --- | --- | --- |
| Re-read required Phase 0/design evidence; preserve candidate | DONE; original 124 paths present | [full manifest](phase0b-files.txt) |
| Genuine identity + stable distinct App/daemon/probe identifiers | preflight/preparation smoke PASS; genuine tree BLOCKED | [signing](phase0b-signing.md) |
| Actual SMAppService register/status/launch/App quit/crash/Store/IPC/reconnect/unregister | signed acceptance UNTESTED; unsigned register refusal PASS | [lifecycle](phase0b-launchagent.md) |
| Controlled signed A/B/C/D/E ScreenCapture/AX/listen tap; Files & Folders separately | UNTESTED; F2 NOT CLOSED; direct C instrumentation still required | [matrix](phase0b-tcc.md) |
| Public responsibility contingency and conditional amendment | investigation DONE; CONTINGENCY_NOT_PUBLICLY_SUPPORTED; no amendment applied | [public API evidence](phase0b-responsibility.md) |
| Protected-root deadline and persistence decision | independent mode/ordinary boundaries PASS; protected-root timeout/persistence signed acceptance UNTESTED | [roots](phase0b-protected-roots.md) |
| Real lock/saver/sleep/wake/FUS | BLOCKED_BY_F2; no transition triggered | [downstream gates](phase0b-lock-input.md) |
| Reliable physical/own/third-party input and Secure Input | BLOCKED_BY_F2; computer.input unavailable | [downstream gates](phase0b-lock-input.md) |
| Actual ChatGPT Tunnel native command | BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS; historical local PASS unchanged | [external checks](phase0b-tunnel-widget.md) |
| Actual widget token/UI approval resumes original operation | BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS; no real E2E PASS | [widget requirements](phase0b-tunnel-widget.md) |
| Required Go/Swift/shell/whitespace checks and smoke | PASS for exercised paths; genuine codesign acceptance blocked | [raw gates](phase0b-gates.txt), [actual smoke](phase0b-smoke.json) |
| Bilingual conclusions/files/commit delivery | DONE; no Complete Phase 0 commit, no push | this document and [manifest](phase0b-files.txt) |

## Incremental changes

- Signing preflight precedes build/output mutation; verifies an actual Apple-issued identity/private key and actual Team rather than name-derived Team. Fixed probe signing ID to com.codebridge.probe. Signed positive path is **UNTESTED** on this host.
- Signed Info.plist/LaunchAgent carry reciprocal App/daemon identity expectations; live peer authentication is unchanged. Unsigned-dev marker/build kind now resolve correctly and unsigned builds have no trusted Team.
- Shared existing probe implementation supports an actual App **in-process** A entry point; old App child remains B. No second permission implementation was introduced.
- SMAppService acceptance CLI and register fail-closed gate verify genuine App plus nested identities. Read-only service status is available without registration.
- Separate Files & Folders mode avoids Computer API calls; timeout text no longer claims an unresolved call proves TCC denial. No production ProjectRegistration, Computer engine, InputArbiter, WebRTC, orchestration or cloud implementation.
- Bilingual roadmap/index and architecture evidence qualification updated without changing frozen F2 or erasing historical evidence.

## Exercised verification

| Command/scenario | Observed result |
| --- | --- |
| make fmt-check | PASS |
| go test ./... | PASS — 17 packages, 8 no tests |
| go vet ./... | PASS |
| go build ./... | PASS |
| make build-daemon | PASS — actual CGO-enabled codebridged binary |
| swift build -c release --package-path desktop/macos | PASS after correcting shared-module import/source ownership |
| swift test --package-path desktop/macos | PASS — 33 tests, zero failures |
| bash -n on both signing scripts | PASS |
| git diff --check | PASS |
| Real builder, missing identity, existing owned output | expected exit 2; SIGNED_TCC_BLOCKED_EXTERNAL; marker preserved |
| Owned unsigned preparation assembly | plutil/marker PASS; not signed/TCC/SMAppService acceptance |
| App --phase0b-probe signing | actual App executable/PID report, exit 0; not a child probe or grant result |
| App --phase0b-service status | actual unsigned status/no loaded job; not signed lifecycle PASS |
| Unsigned register/permissions/input-monitor/files-folders | expected exit 3 before OS acceptance calls |
| Standalone files-folders on owned ordinary roots | readable count=2; missing path; regular-file ENOTDIR; no Computer API report sections |
| Genuine codesign verify/deep/strict, metadata, designated requirements | BLOCKED — no genuine certificate; no result fabricated |

The initial Swift integration attempt failed because the shared CLI lacked its report-module import; target source ownership also warned. Both were corrected, then release build and all tests passed. The initial identity smoke assertion compared /var with /private/var; canonical-path comparison passed using the already captured report, without rerunning application commands. Both facts are retained in raw evidence. Tests alone are not used as OS acceptance proof.

## Files and commits

Baseline 124 candidate files preserved; 18 existing files edited and 15 new files in this pass. Final candidate manifest: **139 files** (9 modified tracked, 130 untracked, 0 staged). Full paths and Phase 0B-only changes: [phase0b-files.txt](phase0b-files.txt). Ignored generated binary/build caches are not counted.

HEAD at scope: `8d1fcdf`. **Commits created: none. Push: none.** No reset/clean, no baseline candidate deletion, no final Complete Phase 0 commit. Only this pass's owned temporary refusal/ordinary-root/unsigned-assembly fixtures were removed. No service was registered by this pass.

## Stopping point

First unblock the genuine certificate/private key and authenticated ChatGPT/Tunnel prerequisites. Then run the explicitly untested signed acceptance matrix, including direct C native calls; do not infer completion from compilation, unsigned refusal smoke or preserved historical local PASS. No further permission/input/OS-transition experiment is run while this external/security gate is open.
