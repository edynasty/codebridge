# Phase 0B — Blocker Closure / Signed Native Acceptance

Date: 2026-10-03. Host: actual macOS 27.0 (26A428), arm64. Baseline: `702bca2`; architecture freeze: `8d1fcdf`.

**PHASE_0_BLOCKED**

- F2: **NOT CLOSED**.
- Phase 1: **NOT READY**. No Phase 1 implementation or Computer input enabled.
- **IDENTITY VALID / SIGNED TREE VERIFIED / SMAppService PASS**: the signing prerequisite is repaired; F2's App-positive/clean-attribution prerequisite is not satisfied.
- **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**: no usable Tunnel credentials/authenticated ChatGPT acceptance surface accessible.
- **CONTINGENCY_NOT_PUBLICLY_SUPPORTED**: responsibility disclaimer has no established supported public implementation; no private API/SPI added.

[中文](phase0b-closeout.zh-CN.md). Historical [Phase 0 closeout](phase0-closeout.md) and exact [TCC attribution](phase0-tcc-attribution.txt) are preserved, not rewritten.

## Current 2026-10-03 closeout

| Required status | Current result |
| --- | --- |
| Code Signing Identity | **VALID** — exact Apple Development target, 1 valid identity |
| Identity | Apple Development: tangxingpeng@hotmail.com (QC2AU8UDY6) |
| Actual approved Team ID | **HXAV5ALQQG**; requested QC2AU8UDY6 was not the certificate OU/TeamIdentifier |
| Trust-chain root cause | missing WWDR G3 intermediate; legacy WWDR expired in 2023; system Apple Root was present |
| Signing smoke | **PASS** — genuine signature, strict verify, actual execution; temporary source/binary removed |
| Signed App/daemon/probe | **PASS** — stable com.codebridge.app / com.codebridge.daemon / com.codebridge.probe, actual Team, Authority, designated requirements and strict/deep verification recorded |
| SMAppService | **PASS** — register, launchd ownership, App quit survival, SIGKILL restart, same Store reopen, signed IPC reconnect, update/re-register and unregister |
| F2 | **NOT CLOSED** — actual App Computer APIs are not ALLOWED; historical override/clean grant causality unresolved; C/D subject is the App |
| Phase 0 | **BLOCKED** — F2 and actual ChatGPT/Tunnel acceptance remain open |
| Phase 1 readiness | **NOT READY** — no production Computer input enabled |

### OBSERVED evidence

- Imported only Apple-official WWDR G3 into login, after built-in trust verification. Did not delete/regenerate the leaf/private key or change Root CA, trust overrides, search list or time policy. [Certificate details and genuine signed metadata](phase0b-signing.md).
- Initial real builder failed because inline `--test-requirement` lacked `=`; fixed both sites and verified the positive path. Initial signed service exited 2 because the plist omitted `run`; unregistered through App, added ProgramArguments and re-tested successfully. Neither failure is hidden.
- Actual daemon PID 35517 survived normal App quit. SIGKILL caused an observed launchd replacement 47519 by the 12s inspection. Old IPC returned EOF; a signed App role=app reconnected with an audit token and verified daemon identity. Same runtime.db path/inode 366202558 and schema 1 reopened. The prior Store was empty, so populated-domain recovery is not claimed. Same-identity rebuild registered PID 50184; final native-report smoke registered PID 60728. Both unregistrations removed the job. [Lifecycle](phase0b-launchagent.md).
- A/B/C/D/E real SCK, AX and listen-only tap calls ran. C is now direct CGO/Objective-C in the daemon PID, not a relabeled D harness. App SCK remained -3801, AX remained -25204/trusted=false and input delivery remained unproven. Shell control delivered 12 physical events with zero injected events. App/daemon restarts and same-identity rebuild did not produce a working App-positive baseline. [Matrix](phase0b-tcc.md).
- TCC C `35517.1` and updated `50184.1` had daemon requesting identity but subject=com.codebridge.app. D `35963.1` had daemon responsible PID 35517/probe requesting PID 35963, but the same App subject. Old App ad-hoc requirement mismatch was recorded. [Exact owned request logs](phase0b-tcc-signed-2026-10-03.txt).
- Later Files & Folders reads were successful in A/B/C/D/E; this does not prove controlled grant causality or protected-root persistence. No TCC database was read/written, no reset/automatic grant performed, and no pixels, input content or directory filenames recorded. Raw GUI inherited environment is excluded.

### DECISION and INFERENCE

- **DECISION:** user explicitly approved genuine Team HXAV5ALQQG after the requested suffix/actual Team conflict was demonstrated. Never fake QC2AU8UDY6 in signed metadata or weaken verification.
- **DECISION:** the user's two manual authorization/cleanup reports are preserved as user-reported actions, not promoted to OS grant/API success. No signed daemon/harness Computer success was observed, so no demonstrated-success-leak ARCHITECTURE_AMENDMENT_REQUIRED result is asserted. No architecture redesign or private disclaimer was applied.
- **INFERENCE:** the observed shared bundled TCC subject is a boundary risk; successful causal App-grant inheritance is not established while A remains non-ALLOWED.

### BLOCKED and minimum next action

1. Establish **effective grants for the current signed App**, demonstrated by actual SCK capture, successful AX API and physical event delivery. Resolve its historical requirement mismatch through human-controlled, targeted CodeBridge-only permission cleanup; any tool-driven reset requires explicit approval and must never target the entire user. Then repeat C/D attribution and the restart/rebuild matrix. If daemon/harness obtains Computer access, report ARCHITECTURE_AMENDMENT_REQUIRED; do not lower F2.
2. Actual ChatGPT/Tunnel credentials/session remain external. Live daemon health reported tunnel `not_configured`, `tunnel binary not configured`; no new ChatGPT/widget E2E PASS is claimed.

### Exercised verification and final state

All requested native checks passed: `make fmt-check`; `go test ./...` (17 packages, 8 no tests); `go vet ./...`; `go build ./...`; `make build-daemon`; `swift build -c release --package-path desktop/macos`; `swift test --package-path desktop/macos` (20 IPC + 13 ComputerSpike tests, zero failures). Final whitespace check is recorded in [raw signed run](phase0b-signed-2026-10-03.json).

Final signed App remains at `desktop/macos/build/CodeBridge.app`; App is quit and the service is **unregistered**. No push, reset/clean, Complete Phase 0 commit or acceptance-closure commit was made. All original Phase 0/0B raw evidence is preserved. The debug-gate regression now also covers native probes; incidental mock-echo assertion removed, caller-argv security assertions retained. [Updated diagnostic contract](../../../schema/hostipc/v1/README.md#7-hostphase0_probe-debug-only).

The remaining sections are the **preserved 2026-10-02 archive**. Their “current”, zero-identity and UNTESTED statements describe that historical pass, not the current status above.


## Historical 2026-10-02 blocking items and evidence

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

## Historical 2026-10-02 stopping point

First unblock the genuine certificate/private key and authenticated ChatGPT/Tunnel prerequisites. Then run the explicitly untested signed acceptance matrix, including direct C native calls; do not infer completion from compilation, unsigned refusal smoke or preserved historical local PASS. No further permission/input/OS-transition experiment is run while this external/security gate is open.

## Latest App-only baseline attempt — 2026-10-03, run 20261003T012940770Z

**APP_TCC_BASELINE: FAIL — APP_TCC_BASELINE_NOT_ESTABLISHED. F2 NOT CLOSED; PHASE_0_BLOCKED; Phase 1 NOT READY.** This appended attempt supersedes no historical evidence: prior matrix/lifecycle results and prior “no reset” statements remain scoped to their earlier runs. This iteration stopped at A and did not rerun B/C/D/E or F2 persistence.

### OBSERVED

- Current bundle: `/Users/tangxingpeng/IdeaProjects/me/codebridge/desktop/macos/build/CodeBridge.app`; identifier `com.codebridge.app`; actual Team **HXAV5ALQQG**. Genuine Apple Development signature, hardened runtime and deep/strict verification passed before reset and after the request-hook rebuild. CDHash changed `4f1bfc56019090b942554a1015f14dd7efbf5f17` → `b7525cda835942ab7cf29360396f4508fc57497b`; the App's designated requirement, identifier and Team stayed the same. [Build, tests and complete signing metadata](phase0b-app-baseline-build-20261003T012940770Z.txt); [exact DR and baseline evidence](phase0b-tcc.md#app-only-positive-baseline--2026-10-03-run-20261003t012940770z--本轮追加).
- Exactly three approved bundle-scoped resets succeeded: `tccutil reset ScreenCapture com.codebridge.app`, `tccutil reset Accessibility com.codebridge.app`, `tccutil reset ListenEvent com.codebridge.app`. The current manual documents bundle scoping but no service list; `ListenEvent` came from this machine's actual TCC request `35517.4` and was confirmed by reset success. No global/other-bundle reset or TCC database read/write was performed.
- Added signed App-only `--phase0b-request-permissions`: public ScreenCapture, AX prompt and ListenEvent requests ran in main-process PID **90964**; all request returns were false. The hook is explicit opt-in; passive probes do not request permission. The user reported **“已仅授权当前 App 并退出”**, preserved as **USER_REPORTED_ACTION_NOT_API_PASS**. [Request receipt](phase0b-app-requests-20261003T012940770Z.json).
- Fresh LaunchServices App processes, parent PID 1 and the same genuine identity, produced these actual acceptance results:

| Failing capability | Observed result | Classification |
| --- | --- | --- |
| Screen Recording, App PID 7361 | Actual SCK SCShareableContent denied **-3801**; capture skipped, no frame/size/hash | FAIL / DENIED |
| Accessibility, App PID 7361 | Actual `AXFocusedApplication` query returned **-25204 / kAXErrorCannotComplete**, trusted=false | FAIL / ERROR |
| Input Monitoring, App PID 8380 | Real listen-only tap installed; **30s, 0 physical events**, preflight=false; input_available=false | FAIL / delivery UNPROVEN, not a denial proof |

- [SCK/AX report](phase0b-app-positive-api-20261003T012940770Z.json); [input report](phase0b-app-positive-input-20261003T012940770Z.json). The tap now includes `.mouseMoved`. No pixels, key contents or injected input; roots were empty, so no Files & Folders experiment was run here.
- Read-only System Settings snapshot: Screen window **录屏与系统录音** has `CodeBridge=1` and separately `CodeBridge.app=0`; Accessibility window **设备控制和数据访问** has App/probe/daemon values 0. The **输入监控** AX tree exposed no CodeBridge references. Display labels do not resolve exact signing identities. No inspector changed a toggle or acquired a new Computer grant, and inspector authority does not count as A evidence.
- TCC matched the current genuine App DR with **status 0**, yet Screen/AX remained **Denied (System Set)**. Actual SCK `60709.3` has requesting replayd PID 60709/accessing App PID 7361, `preflight=no`, subject App, `authValue=0/authReason=4`; AX `7361.2`/`.3` and ListenEvent `8380.1` also return 0/4. [Live logs](phase0b-app-baseline-tcc-20261003T012940770Z.txt); [persisted log-show recovery after live drops](phase0b-app-baseline-log-show-20261003T012940770Z.txt). SystemEvents/VSCode inspector rows remain separate.
- Exercised: Swift release build, **33 tests (20 IPC + 13 ComputerSpike)**, signed rebuild/deep-strict verification, actual App main-process requests and both real probe paths. Go was unchanged/not rerun. No service registration/restart/kill, commit or push in this iteration. [Consolidated raw acceptance record](phase0b-app-baseline-20261003T012940770Z.json).

### INFERENCE

- Entry/path ambiguity between the two Screen labels is a candidate explanation, not a proven user-action error or grant-failure cause. The user action is preserved; effective OS authorization is not established.
- These current DR-matched denial rows are not proof of the earlier ad-hoc requirement mismatch. `anchor apple` platform-only status **-67050** does not invalidate the separately verified Apple Development signature. The system-set denial cause remains unknown; private-TCC and AppleEvents warnings are not proven Computer-grant prerequisites.

### DECISION and minimum next action

- Stop at **APP_TCC_BASELINE_NOT_ESTABLISHED**. B/C/D/E and authorized-A persistence repeats are **NOT RUN** in this attempt. Genuine signing and earlier SMAppService PASS remain intact, not substitutes for a positive App baseline.
- **F2 NOT CLOSED**. Do not infer leakage from `subject=com.codebridge.app` alone. No demonstrated `ARCHITECTURE_AMENDMENT_REQUIRED` result, topology change, private disclaimer/entitlement workaround, signature weakening or additional daemon/probe/parent grant was introduced.
- Minimum next action: human verify the **exact current App path** and corresponding Screen/AX/Input entries; distinguish `CodeBridge` from `CodeBridge.app` without assuming labels are identities. Change only the current App's entries, fully quit/relaunch and repeat A-only real capture, successful AX query and physical tap delivery. Only after all three pass may the native daemon-PID C/D attribution and restart/rebuild acceptance resume.
- ChatGPT/Tunnel/widget work was explicitly excluded. Existing external blockers are preserved, not re-investigated or mixed into this App baseline result.

## Latest GUI-first A-only acceptance — 2026-10-03, run 20261003T023754513Z

**APP_TCC_BASELINE: FAIL — APP_TCC_BASELINE_NOT_ESTABLISHED. F2 NOT CLOSED; PHASE_0_BLOCKED; Phase 1 NOT READY.** Historical signing/lifecycle/matrix results above are retained; this attempt performed only A after human GUI cleanup, with zero resets.

### HUMAN ACTION

Guided removal of historical App entries, exact-path re-addition and App-only enabling on all three pages, without changing other software or granting daemon/probe/harness/parents. The user selected **“三页已清理并授权，两个程序已退出”**, reporting completion and full App/System Settings quit. This report is not OS/API PASS. During the real listen-only observation, mouse movement and one harmless Shift press were requested; the prompt is not itself a hardware-event observation.

### OBSERVED

- Verified current exact bundle `/Users/tangxingpeng/IdeaProjects/me/codebridge/desktop/macos/build/CodeBridge.app`: identifier `com.codebridge.app`, Team **HXAV5ALQQG**, non-ad-hoc Apple Development, deep/strict PASS. CDHash unchanged at `b7525cda835942ab7cf29360396f4508fc57497b`. Same DR, recorded fully in [TCC evidence](phase0b-tcc.md#gui-first-app-only-baseline--2026-10-03-run-20261003t023754513z--gui-优先追加) and [raw acceptance](phase0b-app-gui-baseline-20261003T023754513Z.json). Exact-path App process search before cleanup returned no matches. No rebuild/signature change.
- Fresh LaunchServices App PID **11361**, parent 1: real SCK **DENIED -3801**, capture skipped/no frame; actual AXFocusedApplication **ERROR -25204**, trusted=false. Fresh App PID **13649**, parent 1: real listen-only tap created, **30 seconds / 0 physical events**, preflight=false/input_available=false, no injected/disabled events: **delivery UNPROVEN**, not DENIED. [SCK/AX receipt](phase0b-app-gui-api-20261003T023754513Z.json); [input receipt](phase0b-app-gui-input-20261003T023754513Z.json). No pixels or key contents persisted; roots=[]; no input posted.
- Later read-only actual settings panes: Screen **录屏与系统录音** has CodeBridge=ON, CodeBridge.app=OFF, probe=ON, daemon=ON; AX **设备控制和数据访问** has App/probe/daemon=OFF; **输入监控** exposes no CodeBridge checkbox references in its recorded AX tree. Initial Screen navigation briefly showed the overview; target-pane retry obtained these rows. Display labels do not prove exact identities. The inspector reopened settings only to record evidence and changed no permission toggle; those historical extra-process ON rows were not grants created by this run or authority used to execute C/D.
- Current DR matched status 0 in TCC; Screen/AX remained Denied (System Set). Actual SCK `11093.3`: requesting replayd PID11093/accessing App PID11361, preflight=no, subject App, authValue=0/authReason=4; handling rows responsible App PID11361. AX `11361.2`/`.3`: accessing/requesting/responsible App PID11361, subject App, 0/4. ListenEvent `13649.1`: requesting App PID13649/subject App, 0/4; no unrecorded responsible field inferred. [Live log](phase0b-app-gui-tcc-live-20261003T023754513Z.txt); [exact persisted log-show output after live drops](phase0b-app-gui-tcc-log-show-20261003T023754513Z.txt). Owned recorder stopped.

### INFERENCE

Entry/path/requirement contamination is still only a candidate explanation. Preserve the human report and later observed states separately; do not claim a user-action error or a proven cause. Current DR-matched requests do not demonstrate the earlier ad-hoc mismatch; platform-only -67050 does not invalidate genuine signing, and private-TCC warnings do not justify private entitlements.

### DECISION / minimum next action

- **Stop A acceptance after this failed cycle.** B/C/D/E and F2 restart/rebuild persistence were NOT RUN. F2 remains open; subject App alone is not a leak; no ARCHITECTURE_AMENDMENT_REQUIRED assertion. No Phase 1, code change, build/test rerun, SMAppService mutation, automatic GUI grant, reset, TCC database read/write, private API, signature weakening or extra-process grant. ChatGPT/Tunnel/widget remain outside this iteration.
- Human reconcile the **exact current App path/entries** with the later Screen/AX App-OFF snapshot and reported cleanup; preserve other software, do not grant daemon/probe/harness/parents. Once current App-only state is effective, repeat **A only**. A further reset is not an assumed cure: any future bundle-scoped reset must be necessary, supported, explained and recorded first.

## Unique-label staged acceptance — 2026-10-03, run 20261003T025836514Z

**Stage: Screen — NOT ACCEPTED (LAUNCHSERVICES_TCC_LABEL_STALE). APP_TCC_BASELINE: FAIL; F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.** This pass added identity-preserving instrumentation and produced new evidence: the TCC pane label is stale while LaunchServices and the request attribution are fully current.

### OBSERVED

- `build-app.sh --display-name "CodeBridge Phase0 Signed"` (signed acceptance builds only): bundle ID `com.codebridge.app`, Team **HXAV5ALQQG**, executable `CodeBridge`, daemon/probe identifiers and designated requirement unchanged; new App CDHash `3ea4930142e254fae781f6f2176d167794b05fa2`. `--phase0b-request-permission screen|accessibility|input` requests exactly one permission in the App main process; `screen`/`accessibility` single-capability probes added; AX success now requires a real focused AXUIElement. All checks passed (fmt-check, Go test/vet/build, Swift release build + 33 tests, signed rebuild deep/strict, `git diff --check`). [Build log](phase0b-app-named-build-20261003T025836514Z.txt); [run record](phase0b-app-named-baseline-20261003T025836514Z.json).
- Exact-path App PID **33577** (parent 1) issued the screen-only request: receipt `displayName=CodeBridge Phase0 Signed`, `com.codebridge.app`, Team **HXAV5ALQQG**, `screenRequestReturned=false`. TCC request `33577.3` (actual ScreenCapture, preflight=no): requesting/subject `com.codebridge.app`, `authValue=0, authReason=4`. [Receipt](phase0b-app-screen-request-20261003T025836514Z.json).
- `lsregister -dump` (read-only): `localizedShortNames=CodeBridge Phase0 Signed`, `trustedCodeSignatures` matches the current CDHash. Yet the Screen Recording pane shows only `CodeBridge`, `codebridge-probe`, `CodeBridge.app`, `codebridged` — **the unique label does not appear** → **LAUNCHSERVICES_TCC_LABEL_STALE**.

### INFERENCE

TCC pane labels are frozen at entry creation; they do not track LaunchServices. The stale labels are the historical TCC entries themselves — new direct evidence for entry contamination, not yet proof of the prior failures' root cause.

### DECISION / minimum next action

- No toggle, no TCC.db write, no reset (none approved), no label guessing; Screen/AX/Input acceptance and B/C/D/E remain suspended.
- Human choice: (1) remove the four historical CodeBridge entries on the Screen pane (`−` only, other software untouched), quit System Settings and the App, relaunch the exact bundle with `--phase0b-request-permission screen` to create a fresh uniquely-named entry; or (2) stop here. [TCC evidence](phase0b-tcc.md#unique-label-staged-baseline--2026-10-03-run-20261003t025836514z--唯一显示名追加).

## Screen stage PASS — 2026-10-03, run 20261003T061715868Z

**APP_SCREEN_BASELINE: PASS.** The first staged capability now has a real positive baseline. Accessibility and Input Monitoring were NOT run; B/C/D/E and F2 remain suspended. **F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.**

### OBSERVED

- Exactly one user-approved scoped reset was executed (`tccutil reset ScreenCapture com.codebridge.app`, exit 0) after pre-reset identity/quits were recorded. The post-reset screen-only request (App PID 30496) showed the stored denial cleared: `Auth Right: Unknown (None), promptType: 1` — a real authorization opportunity. The user then authorized via the system prompt/Settings (HUMAN ACTION).
- After full quits, fresh exact-path App PID **36351** (parent 1, same bundle/Team/CDHash) ran the real `screen` probe: `SCShareableContent ok`, **real capture ok, 1800×1169, SHA-256 `77b9a798663edd4a085e144b967ee0911bf7007c42081ede32f34a94012bdbf0`, pixels_persisted=false**. TCC `36351.1`: subject `com.codebridge.app`, **`Allowed (System Set), authValue=2`**. [Probe report](phase0b-app-screen-positive-20261003T061715868Z.json); [run record](phase0b-app-screen-reset-20261003T061715868Z.json); [TCC evidence](phase0b-tcc.md#screen-stage-pass-after-approved-scoped-reset--2026-10-03-run-20261003t061715868z--录屏阶段通过追加).

### DECISION

- PASS rests only on the real SCK capture — UI ON, request-true and preflight were never counted alone. Round stops: no AX, no Input, no B/C/D/E, no F2 judgment, no repeat reset.
- Next separate round: Accessibility stage (`--phase0b-request-permission accessibility`, then the real `accessibility` probe requiring a valid focused AXUIElement). A baseline PASS for Screen alone does not close F2 or unblock Phase 0/1.

## Accessibility stage FAIL — 2026-10-03, run 20261003T075114032Z

**APP_ACCESSIBILITY_BASELINE: FAIL — AX_REAL_API_FAILED_AFTER_TRUST.** Screen PASS is preserved untouched. Input Monitoring NOT TESTED THIS ROUND. **F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.**

### OBSERVED

- Preflight identity/quits verified, no rebuild. Accessibility-only request PID 99272 first hit the stored denial (`Denied (System Set), authValue=0, authReason=4, DB Action:None`), then the system opened the real authorization UI; the user granted it and TCC wrote `kTCCServiceAccessibility → Allowed (System Set)` (99358.16, granted=true, current DR). [Request record](phase0b-app-ax-request-20261003T075114032Z.json).
- Fresh post-grant probe PID **8198** (parent 1, same bundle/Team/CDHash): **trusted=true, real `AXFocusedApplication` → -25204 kAXErrorCannotComplete**. TCC handling row for PID 8198: **Allowed (System Set)**. Frontmost: Google Chrome PID 774. [Probe report](phase0b-app-ax-positive-20261003T075114032Z.json); [TCC evidence](phase0b-tcc.md#accessibility-stage-fail--2026-10-03-run-20261003t075114032z--辅助功能阶段失败追加).

### DECISION / minimum next action

- FAIL per the staged standard: only the real AX API counts, and it failed after trust. STOP without Input Monitoring, B/C/D/E, F2 judgment, toggles or resets.
- Next: gather AX/WindowServer diagnostics for the probe PID; test a longer-lived LaunchServices-launched App instance (rather than an immediate-run probe process) doing the same read-only `AXFocusedApplication` query; investigate any second Accessibility decision surface. Do not rerun the identical probe.

## Accessibility stage PASS — 2026-10-03, run 20261003T082537258Z

**APP_ACCESSIBILITY_BASELINE: PASS (TEST_GATE_REFINEMENT).** The prior FAIL (run 20261003T075114032Z) was an instrumentation lifecycle artifact: the immediate-exit probe process could not receive AX run-loop replies. Screen PASS preserved. **F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.**

### OBSERVED

- A new `--phase0b-ax-diagnostic` runs inside a real LaunchServices App main process with a live NSApplication run loop; same-identity rebuild (App CDHash `205ee05ae9600bfe5b6510382bed407b0a9e0458`), all native checks green.
- Three runs (PIDs 30230/30763/31715): trusted=true every time; SystemWide AXFocusedApplication intermittent (-25212 / **ok** / -25212); **Path C succeeded 3/3** — NSWorkspace frontmost PID → AXUIElementCreateApplication → AXRole + AXTitle real reads (errorCode 0) against Chrome twice and VSCode once. TCC stayed `Allowed (System Set)`. [Diagnostic record](phase0b-app-ax-diagnostic-20261003T082537258Z.json).

### DECISION

- **PASS** with the refined acceptance gate: real TCC-protected AX read with reliable frontmost-application verification. Production path: NSWorkspace frontmostApplication → PID → application-scoped AX read. **TEST_GATE_REFINEMENT, not SECURITY_REQUIREMENT_WEAKENING** — trust, real protected AX IPC, and frontmost verification remain mandatory and are all demonstrated; the SystemWide attribute's intermittency is documented as API-path-specific. Next: Input Monitoring baseline.

## Continuous-run closeout — 2026-10-03 (runs 20261003T082537258Z / 20261003T083256604Z)

**APP_TCC_BASELINE: PASS; F2: ARCHITECTURE_AMENDMENT_REQUIRED.** All post-F2 stages halted per §17. **Phase 0 BLOCKED (local gates passed where stated; F2 + external gates open); Phase 1 NOT READY.**

### OBSERVED

- Input Monitoring PASS: request returned true; real listen-only tap observed **1488 physical hardware events** (0 injected, 0 disables, Secure Input off, `verifiedRunning`). [Baseline](phase0b-app-input-baseline-20261003T082537258Z.json).
- F2 C-probe on the launchd-owned signed daemon (PID 38655): **real SCK capture succeeded** (1800×1169) with TCC `Allowed (System Set)`, subject `com.codebridge.app`; AX trusted=true; Input listen preflight=true. The bundled daemon exercises the App's grants. [F2 record](phase0b-f2-matrix-20261003T083256604Z.json); [TCC](phase0b-tcc.md#input-baseline-pass-and-f2-leak--2026-10-03-run-20261003t082537258z--20261003t083256604z--输入基线通过与-f2-泄漏追加).
- SMAppService re-verified incidentally: App register started the launchd-owned daemon; `launchctl print` shows running state.
- Tunnel presence check: `not_configured`, tunnel binary missing, token file present (value never printed), 0 tunnel env vars, no authenticated ChatGPT surface → **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**.

### DECISION

- **ARCHITECTURE_AMENDMENT_REQUIRED** — awaiting the user's choice among: IPC-level boundary redesign accepting bundled-subject sharing; standalone daemon bundle for an independent TCC subject; or an established public responsibility/disclaimer mechanism (none known). Persistence, protected roots, lock/FUS and preemption stages are blocked on this decision, not on technical unknowns.
- No commit, no push, no reset beyond the previously approved scoped ones; historical evidence preserved; roadmap updated (bilingual) to match evidence.
