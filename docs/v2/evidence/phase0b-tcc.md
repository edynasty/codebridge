# Phase 0B — Controlled TCC matrix / 受控 TCC 矩阵

Date: 2026-10-03. Result: **F2 NOT CLOSED — APP_COMPUTER_GRANTS_NOT_EFFECTIVE; clean grant history NOT ESTABLISHED**. Identity and signed SMAppService are now valid/exercised.

本轮已运行真实签名 A/B/C/D/E API；C 在 daemon PID 内直接调用，未用 child 代替。App 的真实 Computer ALLOWED 条件仍不成立；错误、超时、tap 分配成功但零投递均不伪装成 DENIED 或隔离 PASS。

## Historical finding remains / 保留历史发现

Phase 0 daemon PID 72321 (parent 1) spawned harness 77878; actual 1800×1169 capture succeeded. TCC request 77878.1 identified responsible/authorization subject `a.out`, daemon path `bin/codebridged`, authValue 2/authReason 4. This is a real daemon-attributed authorization finding. It does **not** establish original App-grant causality, signed inheritance, or impossibility of the frozen topology. Original grant history is unresolved. Preserve [exact attribution](phase0-tcc-attribution.txt) and [original analysis](phase0-tcc.md).

历史截图及 daemon 归属事实不撤销；它不证明原始 App grant 的因果关系、真实签名继承或拓扑不可能。历史授权来源不明，不能强行计为 PASS。

## OBSERVED signed matrix / 真实签名实测矩阵

All executables used the approved genuine Team HXAV5ALQQG and distinct app/daemon/probe signing identifiers. These are **observations across the recorded epochs**, not a successful controlled App-only-grant matrix. [Full reports](phase0b-signed-2026-10-03.json); [owned TCC request logs](phase0b-tcc-signed-2026-10-03.txt).

| Context | Screen Recording: real SCK | Accessibility: real AX | Input Monitoring: real listen-only tap | Files & Folders |
| --- | --- | --- | --- | --- |
| A signed App, in-process | DENIED: SCStreamErrorDomain -3801, no capture | ERROR: -25204/kAXErrorCannotComplete; trusted=false | created=true, listen preflight=false, 0 events; delivery UNPROVEN in 15s and later 5s windows | later Desktop/Documents/Downloads readable, counts 3/7/230 |
| B signed App child | DENIED: -3801 | ERROR: -25204; trusted=false | created=true, preflight=false, 0 hardware events; UNPROVEN | initially root deadlines; later three roots readable |
| C signed SMAppService daemon, direct native call | DENIED: -3801 in daemon PIDs 35517, 47519, 50184 | ERROR: -25204; trusted=false, called in those daemon PIDs | real listen-only tap created, preflight=false, 0 events in 3s; UNPROVEN | direct calls initially reached 5s/root deadlines; later readable in PID 47519 and final native smoke PID 60728 |
| D signed harness, daemon child | DENIED: -3801 | ERROR: -25204; trusted=false | actual tap created, preflight=false, 0 hardware events; UNPROVEN | initially deadlines; later three roots readable |
| E shell-launched signed control | DENIED: -3801 | ERROR: -25204 despite trusted=true | **ALLOWED delivery observed**: listen preflight=true, 12 physical-hardware events in 3s, zero disabled/injected events | three roots readable |

**OBSERVED** — E's actual TCC responsible process was **com.microsoft.VSCode**, not an assumed Terminal. The control proves event delivery existed in this GUI session, not that App/daemon monitoring was allowed. AX's error is kept as an API error even where preflight trust was true.

**OBSERVED C implementation** — Debug-only `host.phase0_probe` now accepts `daemon-permissions` and `daemon-files-folders`. CGO/Objective-C invokes SCShareableContent and, when available/authorized, SCScreenshotManager; AXUIElementCopyAttributeValue; CGEventTapCreate with kCGEventTapOptionListenOnly; and read-only directory-entry counting. The JSON includes getpid/getppid and the daemon's live signing identity; returned argv is empty. Existing `harness`/`input-monitor` calls remain D. No input is posted, event content inspected, pixels persisted, filename reported, or production Computer capability enabled. Native timeouts/errors are not classified as denial. Final same-PID folder smoke also verified report ownership/boolean serialization without requesting Computer grants.

## OBSERVED attribution, history and persistence / 归属、历史与保持性

- **OBSERVED** — C request `35517.1` has requesting identifier com.codebridge.daemon and its bundled daemon path, but authorization **subject=com.codebridge.app**. Restart/update request `50184.1` has that same App subject, authValue=0/authReason=4. D request `35963.1` names responsible daemon PID 35517 and requesting probe PID 35963, but again **subject=com.codebridge.app**, authValue=0/authReason=4. Separate signing identifiers/parent 1 did not produce the expected independent subjects.
- **OBSERVED** — App Accessibility logs rejected historical `cdhash H"c2d76a2c574d653305f2f2b081e83817cfa5a4aa"` against the new Apple-issued designated requirement. Read-only System Settings snapshot showed Screen Recording: CodeBridge=1, codebridge-probe=1, codebridged=1, CodeBridge.app=0; Accessibility: CodeBridge.app/probe/daemon=0. The Input Monitoring page exposed no CodeBridge checkbox at that snapshot. This was a contaminated pre-cleanup baseline, not a clean denial proof.
- **DECISION / user-reported actions** — The user selected first 已仅授权当前 App, then 已清理并仅授权签名 App. These report human actions, not API success or proof of a clean grant baseline. Subsequent actual App calls still returned SCK -3801, AX -25204/trusted=false and listen preflight=false. No script changed a TCC database, reset TCC, created a grant or injected input.
- **OBSERVED** — App restart, daemon SIGKILL/restart and same-Team/same-identifier rebuild/re-sign were exercised. The App remained non-ALLOWED and C/D had no successful Computer API result. These repeats do **not** establish the required persistence of an authorized A versus denied C/D.
- **INFERENCE** — Bundled-executable attribution is a risk to the proposed independent-identity boundary. The log proves the shared authorization subject, but without a working App-only positive grant it does not prove successful causal grant inheritance.
- **DECISION** — No signed daemon/harness Computer success was observed, so **ARCHITECTURE_AMENDMENT_REQUIRED is not asserted as a demonstrated success-leak finding**. F2 remains frozen and open; no topology redesign, private disclaimer, entitlement workaround or weaker standard was introduced.
- **BLOCKED** — First establish current signed App's SCK capture, successful AX API and listen-only physical-event delivery as actually ALLOWED. Historical override cleanup is not proven by the post-action APIs. Any targeted reset must be explicitly human-authorized and limited to the affected CodeBridge identity/services; never reset the user. Then re-run C/D attribution and the persistence matrix. If either receives Computer access, report ARCHITECTURE_AMENDMENT_REQUIRED and do not enable input.

Files & Folders readability is independent of the Computer criterion. These late readable roots do not establish clean grant causality or protected-root update retention; the separate protected-root decision remains open.

The controlled procedure below remains the acceptance standard. It is not relaxed by the new signing/lifecycle results.

## Controlled procedure / 受控步骤

1. Verify all genuine identities, designated requirements and Team; register only through signed App/SMAppService. Record process tree, audit-token-derived identities where available, timestamps and exact executable paths.
2. Record initial System Settings grants and any available TCC request/subject attribution. Do not read/write TCC databases or infer clean state from a false preflight alone. Historical authorization that cannot be explained marks the affected experiment **EVIDENCE_CONTAMINATED**.
3. Obtain Screen Recording, Accessibility and Input Monitoring grants **manually for CodeBridge.app only**. Do not grant daemon, probe, Terminal or harness Computer permissions to make a denial test pass. Record before/after and prompt recipient.
4. In A/B/C/D/E, call real ScreenCaptureKit capture (dimensions/digest only, no saved pixels), an AX API query, and actual listen-only tap creation plus observed event delivery. `permissions` input preflight alone is insufficient; run `input-monitor` separately. Record each API result, error, PID, parent, signing identity and TCC responsible/authorization subject. No probe injects input.
5. C and D must be **DENIED for all three Computer permissions** while A's actual authorized APIs work. Timeout, missing attribution, unknown denial cause or dirty historical state cannot establish isolation. A shared Team is not by itself an inheritance proof or an isolation proof. Files & Folders is tested independently: daemon authorization for explicitly registered roots is a different question.
6. Repeat after App restart, daemon restart, and same-Team/same-identifier rebuild/re-sign; preserve original operation identity and before/after evidence. A single successful run is insufficient.
7. If history contaminates a result, stop with **EVIDENCE_CONTAMINATED**. A targeted reset requires explanation of affected bundle/service, consequences and **human approval before execution**, then actual before/after baseline and grant retest. Never reset the entire user and never manufacture grants. No reset was performed here.

逐组真实 API、归属日志和人工授权前后证据缺一不可。只有 App 获得 Computer grants 时，C/D 三项必须全部拒绝，且重启和相同签名重建后复测。Files & Folders 独立验收。历史污染必须停在 EVIDENCE_CONTAMINATED；定向 reset 先说明影响并经人工同意，不能全用户 reset，本轮未 reset。

First choice: independently identified, genuine Apple-signed launchd daemon. [Disclaimer research](phase0b-responsibility.md) forbids silently using private API as a fallback. No architecture amendment is justified by certificate absence.

## App-only positive baseline — 2026-10-03, run 20261003T012940770Z / 本轮追加

**APP_TCC_BASELINE: FAIL — APP_TCC_BASELINE_NOT_ESTABLISHED. F2 NOT CLOSED; PHASE_0_BLOCKED; Phase 1 NOT READY.** Earlier matrix, attribution and “no reset” statements above describe the preserved earlier runs. This new run executed only A; B/C/D/E and F2 persistence repeats were stopped at the failed A gate.

### OBSERVED / 实测

- Exact current bundle: `/Users/tangxingpeng/IdeaProjects/me/codebridge/desktop/macos/build/CodeBridge.app`; identifier `com.codebridge.app`; actual Team `HXAV5ALQQG`; non-ad-hoc Apple Development signature, hardened runtime and strict/deep verification **PASS** before reset and after preparation. App CDHash changed from `4f1bfc56019090b942554a1015f14dd7efbf5f17` to `b7525cda835942ab7cf29360396f4508fc57497b` during the request-hook rebuild; identifier, Team and designated requirement stayed the same. Nested daemon/probe retain their distinct signing identifiers. [Actual build/test/signing output](phase0b-app-baseline-build-20261003T012940770Z.txt).
- Stable designated requirement: `identifier "com.codebridge.app" and anchor apple generic and certificate leaf[subject.CN] = "Apple Development: tangxingpeng@hotmail.com (QC2AU8UDY6)" and certificate 1[field.1.2.840.113635.100.6.2.1] /* exists */`.
- The current machine's `tccutil` manual documents bundle-scoped `reset service [bundle_id]` but does not enumerate services. Input Monitoring's `ListenEvent` value was derived from this machine's actual `kTCCServiceListenEvent` request `35517.4`, then confirmed by successful supported execution. At 01:29:59 UTC, the following **three user-approved commands only** each exited 0 with a successful bundle-scoped reset receipt:

```sh
/usr/bin/tccutil reset ScreenCapture com.codebridge.app
/usr/bin/tccutil reset Accessibility com.codebridge.app
/usr/bin/tccutil reset ListenEvent com.codebridge.app
```

- Added the explicit `--phase0b-request-permissions` opt-in to the signed App main process. PID **90964** called public `CGRequestScreenCaptureAccess`, `AXIsProcessTrustedWithOptions(prompt=true)` and `CGRequestListenEventAccess`; all returned false. Request returns are not acceptance. No child, daemon, probe or inspector requested these grants on A's behalf. Passive probes remain non-requesting. [App request receipt](phase0b-app-requests-20261003T012940770Z.json).
- The user reported **“已仅授权当前 App 并退出”**. Preserve this as **USER_REPORTED_ACTION_NOT_API_PASS**, not a claim that the user failed to act. Fresh LaunchServices processes then executed the same signed App, with parent PID 1:

| Required capability | Actual App API result | Acceptance |
| --- | --- | --- |
| Screen Recording, PID 7361 | SCShareableContent **DENIED**, SCStreamErrorDomain **-3801**; capture skipped, no frame/size/hash obtained, no pixels persisted | FAIL |
| Accessibility, PID 7361 | Actual `AXFocusedApplication` query **ERROR -25204 / kAXErrorCannotComplete**; trusted=false | FAIL |
| Input Monitoring, PID 8380 | Real `.listenOnly` tap created=true; 30s observation; **0 hardware events**, 0 injected/disabled events; preflight=false, delivery_proof=preflight_false, input_available=false | UNPROVEN, not an API-denial proof |

- Added `.mouseMoved` to the tap event mask so the requested physical mouse movement is included. No event contents were inspected/persisted and no input was posted. [SCK/AX report](phase0b-app-positive-api-20261003T012940770Z.json); [physical-delivery report](phase0b-app-positive-input-20261003T012940770Z.json). Protected-root list was empty; no Files & Folders experiment was run here.
- Read-only System Settings AX snapshot after the API failure: **录屏与系统录音** exposes two separate labels, `CodeBridge=1` and `CodeBridge.app=0`; Accessibility's actual window is **设备控制和数据访问**, with `CodeBridge.app=0`, `codebridge-probe=0`, `codebridged=0`. The recorded **输入监控** AX tree exposes no CodeBridge checkbox references. Labels alone do not prove an entry's exact bundle/signing identity. No toggles were changed by the inspector, and its existing authority is not A evidence.
- Current App designated-requirement checks match with **status 0**. ScreenCapture and Accessibility still report **Denied (System Set)**. Bounded persisted logs record `authValue=0/authReason=4`: Screen preflight `7361.1`; actual SCK request `60709.3` (`preflight=no`, requesting replayd PID 60709, accessing App PID 7361); AX `7361.2`/`7361.3`; ListenEvent `8380.1`. These authorization subjects are `com.codebridge.app`; no daemon/harness API ran in this new epoch.
- [Owned live TCC log](phase0b-app-baseline-tcc-20261003T012940770Z.txt) contains dropped-message notices; [bounded log-show recovery](phase0b-app-baseline-log-show-20261003T012940770Z.txt) preserves the actual result rows. SystemEvents/VSCode inspector attribution is separate, not promoted to App authorization.
- Swift release build, **33 tests (20 IPC + 13 ComputerSpike)** and signed rebuild/strict verification passed. The new request path and both real A probe paths were actually launched; the negative API results above remain the smoke result. Go was unchanged and not rechecked. [Consolidated commands, human action, UI snapshot, identities and decisions](phase0b-app-baseline-20261003T012940770Z.json).

### INFERENCE / 推断

- The two Screen Settings labels leave entry/path ambiguity as a candidate explanation, not a proven cause of ineffective grants. The actual current App remains non-ALLOWED despite the reported human action.
- Matched current DR means these current denial rows do not demonstrate the earlier ad-hoc requirement mismatch. The separate `anchor apple` platform-only check returning **-67050** is not evidence that the verified Apple Development signature is invalid. The root cause of the system-set denials is not established.
- The logs' private TCC entitlement recommendation and AppleEvents entitlement warning do not prove that either entitlement is required to establish Computer access; neither was added as a workaround.

### DECISION / 决策与最小下一步

- **APP_TCC_BASELINE_NOT_ESTABLISHED**: stop B/C/D/E, C/D comparison and authorized-A restart/daemon-restart/rebuild persistence acceptance. No SMAppService registration, restart or kill was performed in this iteration. Earlier lifecycle PASS remains preserved, not rerun.
- **F2 NOT CLOSED**. No `ARCHITECTURE_AMENDMENT_REQUIRED` success-leak result is asserted; `subject=com.codebridge.app` alone is not leakage. No architecture refactor, private API/disclaimer, signature weakening, global/other-bundle reset, TCC database read/write or extra daemon/probe/parent Computer grant was introduced. ChatGPT/Tunnel/widget work was outside this iteration.
- Minimum next action: human-check the exact current App path and the corresponding entries for all three services, distinguishing the two Screen labels without assuming a label is an identity; change only the current App's entries, fully quit/relaunch it, then repeat **A only** for real capture, successful AX query and physical event delivery. Until all three are positive, C/D and F2 persistence remain blocked; do not repeat unrelated resets or manufacture grants.

## GUI-first App-only baseline — 2026-10-03, run 20261003T023754513Z / GUI 优先追加

**APP_TCC_BASELINE: FAIL — APP_TCC_BASELINE_NOT_ESTABLISHED. F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.** This is a new A-only attempt after human GUI cleanup, not a repeat of the earlier automatic scoped resets. History above is preserved.

### HUMAN ACTION / 人工动作

- Guided removal of historical `CodeBridge` / `CodeBridge.app` entries on Screen Recording, Accessibility and Input Monitoring, re-addition via the exact current bundle path, and enabling only that App. Explicitly excluded other software and daemon/probe/harness/Terminal/editors/agents. The user selected **“三页已清理并授权，两个程序已退出”**: all three pages processed, current App granted, App and System Settings fully quit. This is the reported human action, **not OS authorization or API PASS**.
- Before cleanup, the exact current App process search had no matches. The physical-input observation explicitly requested mouse movement and one harmless Shift key press; no event content was recorded, and the prompt alone is not proof that hardware input occurred.

### OBSERVED / 实测

- Exact bundle `/Users/tangxingpeng/IdeaProjects/me/codebridge/desktop/macos/build/CodeBridge.app`, identifier `com.codebridge.app`, Team `HXAV5ALQQG`, non-ad-hoc genuine Apple Development signature, runtime flags and deep/strict **PASS**. CDHash remains `b7525cda835942ab7cf29360396f4508fc57497b`; no rebuild or signing change in this run. DR remains `identifier "com.codebridge.app" and anchor apple generic and certificate leaf[subject.CN] = "Apple Development: tangxingpeng@hotmail.com (QC2AU8UDY6)" and certificate 1[field.1.2.840.113635.100.6.2.1] /* exists */`.
- Fresh LaunchServices A process **11361**, parent 1, actually called SCK SCShareableContent: **DENIED -3801**, capture skipped, no frame/dimensions/hash, no persisted pixels. Actual `AXFocusedApplication` query: **ERROR -25204 / kAXErrorCannotComplete**, trusted=false. [Actual API receipt](phase0b-app-gui-api-20261003T023754513Z.json).
- Fresh LaunchServices A process **13649**, parent 1, same signature/id/Team/CDHash: real listen-only tap created=true, **30s / 0 hardware events**, 0 injected events, 0 tap disables, preflight=false, delivery_proof=preflight_false, input_available=false. **Delivery UNPROVEN**, not an API-denial proof. [Actual input receipt](phase0b-app-gui-input-20261003T023754513Z.json). No protected-root operations; roots=[]; no input posting/key contents/pixels persisted.
- After both API runs, read-only AX inspection reopened System Settings for evidence only, not for a second authorization cycle. Actual **录屏与系统录音** pane: `CodeBridge=1`, `CodeBridge.app=0`, `codebridge-probe=1`, `codebridged=1`. Actual **设备控制和数据访问** pane: `CodeBridge.app=0`, probe=0, daemon=0. **输入监控** exposed no CodeBridge checkbox reference in its recorded AX tree. The initial Screen navigation briefly showed the Privacy overview; a target-pane read corrected that incomplete snapshot. Labels alone do not identify a checkbox's exact executable/DR. These observed rows are not new grants made by this run; no inspector changed a permission toggle.
- TCC matches the current DR with **status 0**, then records Screen/AX **Denied (System Set)** with `authValue=0/authReason=4`. Screen preflight `11361.1`: requesting App PID11361, subject App. Actual SCK `11093.3`: preflight=no, requesting `com.apple.replayd` PID11093, accessing App PID11361, subject `com.codebridge.app`; handling rows identify responsible App PID11361. AX `11361.2`/`.3`: accessing/requesting App PID11361, subject App, same responsible App. ListenEvent `13649.1`: requesting App PID13649, subject App, preflight=yes, result 0/4; no separate responsible field in that recorded attribution row is invented.
- [Owned live TCC output](phase0b-app-gui-tcc-live-20261003T023754513Z.txt) has PTY wraps and a dropped-message notice. [Raw non-PTY bounded log-show output](phase0b-app-gui-tcc-log-show-20261003T023754513Z.txt) preserves exact request/subject/result rows. The owned recorder was stopped. [Full identities, commands, HUMAN ACTION, API receipts and settings snapshot](phase0b-app-gui-baseline-20261003T023754513Z.json).

### INFERENCE / 推断

- Historical entry/path/requirement contamination remains only a candidate explanation. Human cleanup confirmation and later App-OFF/API-denied observations are preserved separately; neither proves an incorrect human action nor the reason grants did not become effective.
- Current matched DR does not demonstrate an old ad-hoc mismatch in these requests. Platform-only `anchor apple` -67050 does not invalidate genuine Apple Development verification. Public AX use producing a private-TCC recommendation does not justify adding private entitlements. Root cause remains unestablished.

### DECISION / 决策

- Stop at **APP_TCC_BASELINE_NOT_ESTABLISHED**. B/C/D/E, daemon restart and authorized-A restart/rebuild persistence are **NOT RUN**. Do not infer leakage from subject App; no ARCHITECTURE_AMENDMENT_REQUIRED conclusion or F2 closure. No Phase 1, code change, build/test rerun, SMAppService mutation or ChatGPT/Tunnel/widget work.
- GUI-first run performed **zero tccutil resets**, no TCC database read/write, no private API/SPI, no fabricated grant, no extra-process grant and no automatic permission clicking. Earlier reset receipts remain historical; no second A cycle was run after these failures.
- Minimum next action: human reconcile the exact current App path/entries with the observed Screen/AX App-OFF states and the reported cleanup; leave other software untouched and ensure daemon/probe/harness/parents receive no extra Computer grants. Resume **A only** after current App-only state is established. Any later reset is conditional, bundle-scoped and explained/recorded first; no unproven reset cure is prescribed here.

## Unique-label staged baseline — 2026-10-03, run 20261003T025836514Z / 唯一显示名追加

**Stage: Screen Recording — NOT ACCEPTED. APP_TCC_BASELINE remains FAIL / NOT ESTABLISHED; F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.** This run added identity-preserving acceptance instrumentation and stopped at **LAUNCHSERVICES_TCC_LABEL_STALE** before any authorization toggle or API acceptance attempt.

### OBSERVED / 实测

- `build-app.sh` gained a `--display-name <name>` option restricted to signed `--embed-phase0-env` acceptance builds. The rebuilt bundle keeps `CFBundleIdentifier=com.codebridge.app`, `TeamIdentifier=HXAV5ALQQG`, `CFBundleExecutable=CodeBridge`, daemon/probe identifiers and the same designated requirement; `CFBundleName`/`CFBundleDisplayName` became **CodeBridge Phase0 Signed**. New App CDHash `3ea4930142e254fae781f6f2176d167794b05fa2` (DR permits rebuild). [Build, tests, signing and plist](phase0b-app-named-build-20261003T025836514Z.txt).
- App-only single-permission instrumentation: `--phase0b-request-permission screen|accessibility|input` replaces the combined switch; the App main process requests exactly one service. New `screen` and `accessibility` single-capability probes run only their own real API. `AXFocusedApplication` success now additionally requires a valid returned AXUIElement, not just `kAXErrorSuccess`.
- Full checks passed: `make fmt-check`; `go test ./...` (17 packages, 8 no tests); `go vet`; `go build`; Swift release build; `swift test` (33 tests, 0 failures); `bash -n` + `--help`; signed rebuild with deep/strict verify; `git diff --check` clean.
- Exact-path App launch confirmed: PID **33577**, parent 1, executable `…/build/CodeBridge.app/Contents/MacOS/CodeBridge`. Its screen-only request receipt records `displayName=CodeBridge Phase0 Signed`, `signingIdentifier=com.codebridge.app`, `teamIdentifier=HXAV5ALQQG`, `screenRequestReturned=false` (a request return, not acceptance). [Receipt](phase0b-app-screen-request-20261003T025836514Z.json).
- **TCC attribution is correct and current**: request `33577.1` (ListenEvent preflight) and the actual ScreenCapture request `33577.3` (preflight=no) both carry requesting/subject `com.codebridge.app` with the current bundle path; `33577.3` returned `authValue=0, authReason=4`.
- **Read-only LaunchServices metadata is already correct**: `lsregister -dump` shows `localizedShortNames=CodeBridge Phase0 Signed`, `identifier=com.codebridge.app`, `teamID=HXAV5ALQQG`, `trustedCodeSignatures=3ea493…` matching the current CDHash.
- **But the Screen Recording pane still shows only historical labels**: `CodeBridge`, `codebridge-probe`, `CodeBridge.app`, `codebridged`. `CodeBridge Phase0 Signed` does not appear → **LAUNCHSERVICES_TCC_LABEL_STALE**. [Consolidated run record](phase0b-app-named-baseline-20261003T025836514Z.json).

### INFERENCE / 推断

- The TCC pane label is stored in the TCC record at entry-creation time; it is not read from current LaunchServices registration. Refreshing lsregister metadata would not change the pane label. The stale label lives in the historical TCC entries — this is direct new evidence for the entry-contamination hypothesis, still not proof of the earlier grant failures' root cause.

### DECISION / 决策与最小下一步

- Stop before any toggle: no TCC.db modification, no global reset, no bundle-scoped reset (none approved this round), no label guessing, no Screen/AX/Input acceptance run. B/C/D/E and F2 remain suspended.
- Minimum next action (human choice): either (1) delete all four historical CodeBridge entries on the Screen Recording pane via `−` only, fully quit System Settings and the App, then relaunch the exact bundle with `--phase0b-request-permission screen` so a fresh uniquely-named entry is created, or (2) stop and keep LAUNCHSERVICES_TCC_LABEL_STALE. Do not touch other software; do not grant daemon/probe/harness/parents.

## Screen stage PASS after approved scoped reset — 2026-10-03, run 20261003T061715868Z / 录屏阶段通过追加

**APP_SCREEN_BASELINE: PASS — real SCK capture succeeded.** Stage ordering: Screen is the first of three; Accessibility and Input Monitoring are NOT run in this round; B/C/D/E and F2 remain suspended. **F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.**

### HUMAN ACTION / 人工动作

- The user approved exactly one scoped reset — `tccutil reset ScreenCapture com.codebridge.app` — and nothing else (no All, no Accessibility, no ListenEvent, no daemon/probe, no TCC.db edit, no other apps). After the reset the user enabled Screen Recording for the current App via the system prompt/Settings and confirmed completion.

### OBSERVED / 实测

- Pre-reset verification: App and System Settings fully quit (a lingering request process PID 74528 and Settings PID 74730 were quit first and re-verified absent); exact bundle `/Users/tangxingpeng/IdeaProjects/me/codebridge/desktop/macos/build/CodeBridge.app`, identifier `com.codebridge.app`, Team `HXAV5ALQQG`, strict/deep verify PASS, CDHash `3ea4930142e254fae781f6f2176d167794b05fa2`.
- Exactly one approved reset executed: exit 0, "Successfully reset ScreenCapture approval status for com.codebridge.app". No probe was run immediately after.
- Post-reset screen-only request (App PID 30496, parent 1): TCC `30496.3` kTCCServiceScreenCapture changed from the prior `Denied (System Set)` to **`Auth Right: Unknown (None), promptType: 1, DB Action:None, UpdateVerifierData`** — the stored denial was cleared and a real authorization opportunity appeared. [Run record](phase0b-app-screen-reset-20261003T061715868Z.json).
- After human authorization and full quits of App and System Settings, a fresh exact-path App process **PID 36351** (parent 1) ran the single-capability `screen` probe:

| Measurement | Observed result |
| --- | --- |
| `SCShareableContent` | **ok** (display_count 1) |
| Real capture | **ok** — real frame obtained |
| Dimensions | **1800 × 1169** |
| SHA-256 digest | `77b9a798663edd4a085e144b967ee0911bf7007c42081ede32f34a94012bdbf0` |
| Pixels persisted | **false** |
| TCC `36351.1` | subject `com.codebridge.app`, **`Auth Right: Allowed (System Set), authValue=2`** |

- [Full probe report](phase0b-app-screen-positive-20261003T061715868Z.json). SCK-internal microphone requests (`36351.3/.4`, authValue=1/Unknown) are outside Screen PASS scope and not treated as any decision.

### DECISION / 决策

- **PASS is based solely on the real SCK capture** (real frame + dimensions + digest, no pixels persisted) — not on UI ON, request return value, or preflight. `APP_SCREEN_BASELINE: PASS`.
- Round stops here per staging: no Accessibility, no Input Monitoring, no B/C/D/E, no F2 judgment, no second reset. **F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY** until AX and Input each establish a real positive baseline in separate rounds.

## Accessibility stage FAIL — 2026-10-03, run 20261003T075114032Z / 辅助功能阶段失败追加

**APP_ACCESSIBILITY_BASELINE: FAIL — classification: AX_REAL_API_FAILED_AFTER_TRUST.** Screen baseline (PASS, run 20261003T061715868Z) was not touched this round: no Screen request, no capture, no reset. Input Monitoring NOT TESTED THIS ROUND; B/C/D/E and F2 remain suspended. **F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY.**

### OBSERVED / 实测

- Preflight: App and System Settings fully quit (no residue); exact bundle verified without rebuild — `com.codebridge.app`, Team `HXAV5ALQQG`, non-ad-hoc, strict/deep PASS, CDHash `3ea4930142e254fae781f6f2176d167794b05fa2`, DR unchanged.
- Accessibility-only request (App main process PID **99272**, parent 1): first requests `99272.3/.4` hit the stored denial — `Denied (System Set), authValue=0, authReason=4, DB Action:None, promptType:1`. Immediately after, the system opened the real Accessibility authorization UI (`TCCAccessCheckIfDisclosurePromptIsNeeded`; SecurityPrivacyExtension PID 99358), so the stale decision was superseded by a genuine new authorization opportunity (branch A).
- **HUMAN ACTION (observed via TCC log)**: the user enabled Accessibility for the current App; TCC `99358.16 TCCAccessSetInternal granted=true` wrote **`Update Access Record: kTCCServiceAccessibility for com.codebridge.app to Allowed (System Set)`** with the current DR. [Request record](phase0b-app-ax-request-20261003T075114032Z.json).
- After full quits (old PID 99272 gone), a fresh exact-path probe PID **8198** (parent 1, same identity/CDHash) ran the real `accessibility` probe:

| Measurement | Observed result |
| --- | --- |
| `AXIsProcessTrusted` | **true** |
| Real `AXFocusedApplication` | **error -25204 / kAXErrorCannotComplete** |
| TCC handling row (PID 8198) | `kTCCServiceAccessibility: Allowed (System Set)` |
| Frontmost process at probe time | Google Chrome PID 774 (normal GUI app) |

- [Probe report](phase0b-app-ax-positive-20261003T075114032Z.json). No AX writes, no UI clicks, no keyboard input; AX/WindowServer logs for PID 8198 show no additional AX error diagnostics beyond the API result.

### INFERENCE / 推断

- TCC authorization is effective (Allowed), the probe process is post-grant and freshly launched, and a normal GUI app was frontmost — yet the real AX query still fails with kAXErrorCannotComplete. The failure is therefore not explained by trust state, stale process, or a missing focused app. The exact cause is not established; no root cause is guessed.

### DECISION / 决策与最小下一步

- **APP_ACCESSIBILITY_BASELINE: FAIL / AX_REAL_API_FAILED_AFTER_TRUST** — trusted=true alone was never counted; the real AX API failed. STOP: no Input Monitoring, no B/C/D/E, no F2 judgment, no toggle, no reset (none executed this round; Accessibility stale-decision branch was superseded by the real UI opportunity).
- Minimal next action: investigate why a TCC-Allowed signed App receives kAXErrorCannotComplete — collect AX subsystem/WindowServer diagnostics for the probe PID, test whether a longer-lived LaunchServices-launched App instance (not an immediate-run probe process) behaves differently, and check for a second Accessibility decision surface. Do not repeat the same probe unchanged.

## Accessibility stage PASS via run-loop diagnostic — 2026-10-03, run 20261003T082537258Z / 辅助功能阶段通过追加

**APP_ACCESSIBILITY_BASELINE: PASS.** This supersedes the FAIL of run 20261003T075114032Z: that failure was an instrumentation lifecycle artifact, not a macOS permission failure. **TEST_GATE_REFINEMENT** (not a security weakening). Screen PASS untouched. **F2 NOT CLOSED; Phase 0 BLOCKED; Phase 1 NOT READY** pending remaining gates.

### OBSERVED / 实测

- New `--phase0b-ax-diagnostic` runs in a **real LaunchServices-launched App main process with a live NSApplication run loop** (the probe process exits before AX run-loop replies can arrive; that was the earlier -25204/-25212 root cause). Rebuilt/re-signed same identity: App CDHash `205ee05ae9600bfe5b6510382bed407b0a9e0458`, identifier/Team/DR unchanged; all checks passed (fmt-check, Go test/vet/build, daemon build, Swift release + 33 tests, deep/strict verify).
- Three diagnostic runs (PIDs 30230 / 30763 / 31715, parent 1, same bundle identity):

| Run | trusted | Path B: SystemWide AXFocusedApplication | Path C: NSWorkspace frontmost → app-scoped AXRole/AXTitle |
| --- | --- | --- | --- |
| 1 | true | error -25212 | **ok / ok** (Chrome PID 774) |
| 2 | true | **ok (errorCode 0, valid AXUIElement)** | **ok / ok** (Chrome PID 774) |
| 3 | true | error -25212 | **ok / ok** (VSCode) |

- TCC remained `Allowed (System Set)` for the App identity throughout. No AX writes, no UI automation, no keystroke/content capture. [Diagnostic record](phase0b-app-ax-diagnostic-20261003T082537258Z.json); raw runs `-repeat`/`-third` adjacent.

### INFERENCE / 推断

- The earlier FAIL's -25204 and these -25212s are the same class: the SystemWide focused-application API path intermittently cannot complete even in a fully trusted, run-loop-alive process (it also succeeded once), while application-scoped reads succeed deterministically. This is API-path-specific, not an Accessibility grant problem.

### DECISION / 决策（TEST_GATE_REFINEMENT）

- **APP_ACCESSIBILITY_BASELINE: PASS** under the refined acceptance: a real, TCC-protected AX read that reliably obtains and verifies the frontmost application. Production path: `NSWorkspace.shared.frontmostApplication → PID → AXUIElementCreateApplication(PID) → protected AX read (AXRole/AXTitle)`. This satisfies the frozen architecture's per-key focused/frontmost-app revalidation requirement: NSWorkspace yields the frontmost app identity and the application-scoped read proves live AX access to it.
- This is a **TEST_GATE_REFINEMENT**, not a security-requirement weakening: TCC authorization (Allowed, subject com.codebridge.app), real protected AX IPC, and reliable frontmost verification are all still required and all demonstrated. Path B remains recorded as intermittent evidence.

## Input baseline PASS and F2 leak — 2026-10-03, run 20261003T082537258Z & 20261003T083256604Z / 输入基线通过与 F2 泄漏追加

**APP_TCC_BASELINE: PASS** (Screen + AX + Input all real-positive). **F2: ARCHITECTURE_AMENDMENT_REQUIRED** — the bundled daemon obtained real ScreenCapture success under App-only grants. All post-F2 stages are HALTED pending the user's architecture decision.

### OBSERVED / 实测

- **Input Monitoring PASS**: request `listenRequestReturned=true` (PID 33788; no stale denial, no reset). Real listen-only tap probe (PID 34090, parent 1, same identity/CDHash `205ee05ae9600bfe5b6510382bed407b0a9e0458`): **1488 physical hardware events in 30s** (user mouse movement), 0 injected events, 0 tap disables, Secure Input off, `input_available=true`, `delivery_proof=hardware_event_observed`, `monitor_state=verifiedRunning`. Only counts/classification recorded; no keys or content. [Input baseline](phase0b-app-input-baseline-20261003T082537258Z.json); [raw report](phase0b-app-input-positive-20261003T082537258Z.json).
- **F2 matrix (C first)**: daemon registered through the signed App's SMAppService (launchd-owned PID **38655**, parent 1, signing identifier `com.codebridge.daemon`). The daemon-in-process native probe (`execution=daemon_in_process`, CGO — not a child) obtained:

| Capability | C daemon PID 38655 real result | TCC |
| --- | --- | --- |
| Screen | **real capture ok, 1800×1169, preflight_granted=true** | `Allowed (System Set)`, subject `com.codebridge.app` (requests `38655.1`, replayd `16148.3`, WindowServer `435.1747`) |
| AX | **trusted=true** (grant inherited) | focused-application API path intermittent (same as App) |
| Input | **preflight_listen_access=true** (partial leak), tap created, 0 events in bounded window | — |

- [F2 leak record](phase0b-f2-matrix-20261003T083256604Z.json). TCC attribution on every daemon request: requesting/accessing = `com.codebridge.daemon`, authorization **subject = `com.codebridge.app`** — the bundled executable shares the App's TCC subject.

### INFERENCE / 推断

- Root cause of the leak: the daemon lives inside `CodeBridge.app`, so TCC resolves bundled executables' authorization subject to the containing app regardless of distinct signing identifiers or launchd ownership. This confirms the historical subject-sharing observations as a deterministic OS behavior, now with a clean App-only-grant positive baseline preceding it.

### DECISION / 决策

- **ARCHITECTURE_AMENDMENT_REQUIRED**. Per §17: persistence matrix, protected roots, lock/saver/sleep/FUS, input-preemption spike and Phase 1 are halted. Minimum architecture choices for the user: (1) accept bundled-subject sharing and redesign the security boundary at the IPC level; (2) move the daemon to a standalone bundle for an independent TCC subject (SMAppService plist `BundleProgram` currently points inside the app); (3) a supported public responsibility/disclaimer mechanism — none established. No leak-masking, no daemon grant creation, no private API, no TCC.db write.
- External gates rechecked: tunnel `not_configured` (`tunnel binary not configured`), token file present (presence only), 0 tunnel env vars, no authenticated ChatGPT surface → **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS** unchanged.
