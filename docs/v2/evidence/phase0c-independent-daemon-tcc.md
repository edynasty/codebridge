# Phase 0C — Independent Daemon TCC Spike / 独立 Daemon TCC 边界验证

Date: 2026-10-03. Run `20261003T084313615Z`. Result: **INDEPENDENT_DAEMON_TCC_BOUNDARY: PASS; F2_TOPOLOGY_SPIKE: PASS_WITH_D_SCREEN_CONTAMINATION (D's Screen row was taken after the user explicitly granted Screen Recording to the external daemon's own entry — not an App-grant inheritance; the clean D Screen=DENIED row must be re-established in the production F2 matrix); Architecture amendment (option b) RECOMMENDED.**

Amendment cause is OBSERVED, not theoretical: in the bundled topology the launchd-owned `com.codebridge.daemon` captured the screen for real under `Allowed (System Set)` with authorization **subject com.codebridge.app** ([F2 leak](phase0b-f2-matrix-20261003T083256604Z.json), [TCC](phase0b-tcc.md)). This spike tested whether a daemon physically outside `CodeBridge.app` escapes that attribution.

## Setup / 实验设置

- Bundled SMAppService job unregistered first (no dual daemons). App Computer baselines (Screen/AX/Input PASS) were not reset or re-granted.
- External install: `~/Library/Application Support/CodeBridge/Phase0C/bin/codebridged` (realpath outside `CodeBridge.app`; verified). Signing unchanged: identifier `com.codebridge.daemon`, Team `HXAV5ALQQG`, CDHash `eb401909883fd5f502a5600be6e5f7a4a9355603`, same DR, strict/deep verify PASS.
- Independent user LaunchAgent `com.codebridge.phase0c.daemon` (`~/Library/LaunchAgents/`), bootstrapped via `launchctl bootstrap gui/501`; `KeepAlive.SuccessfulExit=false`, `RunAtLoad`, phase0 debug env pointing at an external probe copy. Daemon PID **47186, PPID 1** — launchd-owned, process tree exactly `launchd → external codebridged`. This is a Phase 0 spike via manual launchctl, not a production lifecycle acceptance.

## OBSERVED — C (external daemon, in-process) / 外部 daemon 进程内

At the first probe window (16:43:14), before any grant to the external subject:

| Capability | Real result (daemon PID 47186, `execution=daemon_in_process`) | TCC |
| --- | --- | --- |
| Screen | `SCShareableContent` **denied −3801**, capture skipped, preflight false | `Unknown (None)` → records written **Denied (System Set)** for the external path |
| AX | `trusted=false`, −25204 | `Unknown (None)`; no trust inheritance |
| Input | `preflight_listen_access=false`, tap created, 0 events | `Unknown (None)` |

**TCC subject for every request: `/Users/…/Phase0C/bin/codebridged` — the daemon's own path, never `com.codebridge.app`.** The App's grant records (`auth_value=2, com.codebridge.app`) were read side-by-side in the same log window and were NOT used for any daemon request.

## OBSERVED — the grant anomaly / 授权异常

At 16:43:27 — between the C probe and the D harness — `SecurityPrivacyExtension` (System Settings, PID 48048) wrote `TCCAccessSetInternal` → **`Update Access Record: kTCCServiceScreenCapture for the external path to Allowed (System Set)`**. **HUMAN ACTION (user-confirmed): the system prompt triggered by the C probe's SCK call appeared, and the user clicked Allow.** This is a grant to the external daemon's own subject via the Settings UI — not an App-grant inheritance, not a script write.

## OBSERVED — D (probe child of external daemon) / 外部 daemon 子进程

Signed `codebridge-probe` PID 48501, parent 47186, `launch_context=codebridged_child`:

| Capability | Real result | TCC |
| --- | --- | --- |
| Screen | capture ok (1800×1169) — **via the user's explicit grant to the external subject at 16:43:27** | subject = external daemon path, `Allowed (System Set)` |
| AX | `trusted=false`, −25204 | denied |
| Input | `preflight_listen_access=false` | denied |

Attribution: responsible `com.codebridge.daemon pid 47186` (external path), requesting `com.codebridge.probe pid 48501`, subject the external path. No `com.codebridge.app` attribution anywhere.

## Judgment / 判定

- **INDEPENDENT_DAEMON_TCC_BOUNDARY: PASS** — before any grant, the external launchd-owned daemon was denied on all three Computer services under its own path-based TCC subject. No App-grant inheritance.
- **F2_TOPOLOGY_SPIKE: PASS_WITH_D_SCREEN_CONTAMINATION** — D no longer inherits the App's grants (responsible/subject = external daemon, never com.codebridge.app), and D inherits the external daemon's *own* grant; the D-Screen Allowed row traces to the user's explicit Allow for the external entry at 16:43:27, so it is contaminated for isolation purposes. AX and Input stayed denied for C and D throughout; the pre-grant Screen-DENY state was directly observed at C. A clean D Screen=DENIED row under App-only grants is still required from the production F2 matrix. An external daemon Computer grant is itself a dangerous configuration — codified as the production invariant `DAEMON_COMPUTER_TCC_MUST_BE_NONE`.
- **INFERENCE** — external-path + launchd ownership is the OS-level determinant of the TCC subject; the signing identifier alone (com.codebridge.daemon) is not. The spike also surfaced a production requirement: a daemon-side SCK call triggers a real user prompt, so the daemon must never request Computer permissions and IPC policy must fail closed on daemon-side Computer capability.
- **DECISION** — **ARCHITECTURE_AMENDMENT_V2_F2 RECOMMENDED** (option b). SMAppService lifecycle acceptance is now **HISTORICAL_PASS_FOR_BUNDLED_TOPOLOGY**; the external topology requires a fresh lifecycle acceptance (install, launchd start, App-quit survival, SIGKILL restart, IPC reconnect, store reopen, update, uninstall) and a full F2 matrix + persistence re-run before F2 can be CLOSED.

Production lifecycle direction: per-user, root-free, launchd-owned external signed daemon + user LaunchAgent; installer/updater design to be specified separately; no SMAppService assumption for the daemon, no Manager/Docker.

## Raw / 原始证据

[Full spike record](phase0c-independent-daemon-20261003T084313615Z.json) (identity, plist, TCC rows, judgments). Live TCC stream captured during the run; Screen pane snapshot after the anomaly shows five CodeBridge entries with both `codebridged` rows ON (bundled historical + new external grant).
