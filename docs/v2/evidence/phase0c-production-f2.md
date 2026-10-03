# Phase 0C — Production Independent Daemon & F2 Closure / 生产独立 Daemon 与 F2 关闭

Date: 2026-10-03. **F2: CLOSED.** Topology: `INDEPENDENT_DAEMON_PRODUCTION`. [Full matrix record](phase0c-production-f2-20261003T104731270Z.json); [spike history](phase0c-independent-daemon-tcc.md).

## Architecture / 架构

```
CodeBridge.app (TCC-privileged Computer host)
  │ install/update/control (DaemonInstaller, SecStaticCode-verified payload)
  ▼
~/Library/Application Support/CodeBridge/bin/codebridged  (external, signed)
  ▼ launchd user LaunchAgent (~/Library/LaunchAgents/com.codebridge.daemon.plist)
launchd → codebridged (PPID 1)
```

## OBSERVED — Implementation / 实现

- `build-app.sh` now copies the **signed** nested daemon (post-`codesign --identifier com.codebridge.daemon`) into `Contents/Resources/DaemonPayload/` — payload carries the genuine signature (CDHash `7fe900be…`, Team `HXAV5ALQQG`), never the unsigned Go output. Initial unsigned-payload defect was caught by install verification and fixed before any launchd use.
- `DaemonInstaller.swift` (App): payload+staging+destination `SecStaticCode`/`SecCodeCheckValidity` verification, atomic staging→install, plist generation with `plutil -lint`, strict-argv `launchctl` encapsulation with timeout, rollback. Menu entries install/uninstall.
- `guard.go` + native `cb_daemon_passive_tcc` (daemon): `DAEMON_COMPUTER_TCC_MUST_BE_NONE` invariant — passive-only checks (CGPreflight/AXIsProcessTrusted/CGPreflightListen), never requests/prompts; `ComputerTCCBoundaryCheck()` at runtime boundaries; health reports `computer_tcc.safe` and degrades on violation. **Verified live**: launchd-owned daemon reports `safe=true` with all three preflits false; a shell-context daemon correctly reported unsafe and logged `UNSAFE_DAEMON_COMPUTER_PERMISSION`.
- Production gate verified: `host.phase0_probe` answers `-32601 unsupported` without `CODEBRIDGE_PHASE0_DEBUG` (matrix run with the gate enabled via plist env).
- Host IPC peer auth now refuses a verified daemon whose executable still lives inside the app bundle (legacy bundled topology).

## OBSERVED — Lifecycle acceptance / 生命周期验收

| Step | Result |
| --- | --- |
| Install (verify → stage → atomic) | PASS |
| launchd bootstrap & start | PASS |
| PPID = 1 / external realpath | PASS (PIDs 54848/57832/58599/60758) |
| Health + Host IPC | PASS (status ok, store wired) |
| SIGKILL → launchd restart | PASS (54848 → 57832) |
| Store same runtime.db reopen | PASS (schema 1) |
| Update (atomic replace + kickstart) | PASS (→ 58599, identity preserved) |
| App-quit survival | PASS (daemon spanned periods with no App running) |
| DAEMON_COMPUTER_TCC_MUST_BE_NONE | PASS |

## OBSERVED — Clean F2 matrix (App-only grants, daemon no grants) / 干净 F2 矩阵

| | Screen | AX | Input | TCC subject |
| --- | --- | --- | --- | --- |
| **A** App | **ALLOWED** (real capture 1800×1169, PID 65521 post-rebuild) | **ALLOWED** (trusted) | **ALLOWED** (preflight true) | com.codebridge.app |
| **C** external daemon PID 60758, in-process | **DENIED** (−3801) | **DENIED** (trusted=false) | **DENIED** (preflight=false) | external daemon own path |
| **D** probe child PID 64234 (parent 60758) | **DENIED** | **DENIED** | **DENIED** | external daemon path (responsible=external daemon, never com.codebridge.app) |

The Phase 0C D-Screen contamination is resolved: production **D-Screen = denied** under clean conditions.

## OBSERVED — Persistence / 持久性

- App restart/rebuild: A stayed ALLOWED (Screen capture ok, AX trusted, Input preflight) after App re-sign; C/D DENIED throughout.
- Daemon SIGKILL restart and atomic update: identity and denials preserved (matrix ran on post-restart/update PID).
- **F2_PERSISTENCE: PASS.**

## DECISION

**F2: CLOSED.** The external independent daemon establishes the OS-enforced Computer TCC boundary: only CodeBridge.app holds Computer grants; the daemon and its children are denied on all three services with their own path-based subjects. Residual recorded (not reset): stale Screen grants for the deleted bundle-path and Phase0C-path daemon entries remain in TCC; harmless (paths removed), user may remove manually.

## Phase 0 remaining / 后续

F2 closed → next gates: protected roots, lock/saver/sleep/FUS, input preemption (each needs human action), then external ChatGPT/Tunnel. Phase 0 stays **BLOCKED** on external credentials; local gates advanced.
