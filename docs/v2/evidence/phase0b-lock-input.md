# Phase 0B — Lock / Input Monitoring downstream gates

Result: **BLOCKED_BY_F2** and **SIGNED_TCC_BLOCKED_EXTERNAL**. Computer input remains **unavailable (fail closed)**. No physical input was captured, synthetic input injected, machine locked/slept or user switched by this pass.

结果：F2 未关闭且缺签名；computer.input 保持 unavailable。本轮未采集物理输入、注入 synthetic 输入、锁机、休眠或切换用户。

## Acceptance after F2 closes / F2 关闭后验收

| OS scenario | ScreenCapture behavior | CGEvent/queued-input behavior | Current |
| --- | --- | --- | --- |
| screen lock → unlock | actual API result/frame freshness before/during/after | observe no pre-lock action replay | UNTESTED |
| saver enter → exit | actual distinction from lock | suspension/control epoch | UNTESTED |
| sleep → wake | actual interruption/recovery | pending input discarded, fresh observation required | UNTESTED |
| fast user switch → return | actual console-session ownership | no execution in another session | UNTESTED |

User triggers real transitions manually only after signed F2 isolation passes. Collect timestamps, API results, actual OS notifications, queue/controller epoch and fresh-frame acquisition; keep state-model transitions separate from OS evidence. Existing model tests are not proof of delivery or non-replay of CGEvents. No production InputArbiter or injector was introduced.

人工触发真实状态转换；记录真实 API/通知与状态、fresh frame。现有模型测试不能证明 CGEvent 实际不重放。本轮不实现生产 InputArbiter 或 injector。

Input Monitoring acceptance requires real listen-only tap creation **and event delivery**. Its separate matrix must distinguish physical keyboard/mouse, own synthetic events, third-party synthetic events, unknown-source events and Secure Input enabled/disabled. Report source PID/userData classification and counters only, never keys/text. Reliable physical preemption must be observed; self markers, successful tap allocation, preflight=true or model tests alone cannot make input available. Tap disable, unknown attribution, Secure Input or unreliable delivery keeps it unavailable. Resume must be a human action after a fresh observation.

Input Monitoring 要证明真实 listen-only tap 创建及事件投递，并区分物理、自身 synthetic、第三方 synthetic、unknown 与 Secure Input。只记来源分类/计数，不记录按键内容。可靠物理抢占未实测前，一律不可用；只有人类在 fresh observation 后能恢复。

Historical evidence stays separate: [lock-state](phase0-lock-state.md), [input-monitor](phase0-input-monitor.md).

## Lock-state acceptance results — 2026-10-03 / 状态转换验收结果

All scenarios ran against the production independent-daemon topology (F2 CLOSED, daemon guard safe throughout). Real transitions were human-triggered; the observer was an assertion-free LaunchServices probe process. No pixels, keystrokes or input were injected; `computer.input` remains production FAIL CLOSED.

| Scenario | Verdict | Key OBSERVED evidence |
| --- | --- | --- |
| **LOCK_UNLOCK** | **PASS** | initial snapshot `screen_locked=true`; `screenIsUnlocked` @13:42:26; model: controller→none, epoch advanced, `requires_human_resume`; P0 fixture discarded (`queue_replay_blocked=true`); fresh F1 @13:45:43 (digest ≠ F0, after unlock). [Record](phase0c-lock-unlock-20261003.json) |
| **SCREEN_SAVER** | **PASS_AS_LOCK_COUPLED** | only `screenIsLocked/Unlocked` ×2 (13:48:57–13:49:21), zero saver-specific notifications — this host couples saver with lock; fail-closed lock path applies. Fresh frame after exit. [Record](phase0c-screen-saver-20261003.json) |
| **DISPLAY_OFF_AWAKE** (separate state, user-requested modeling) | **MODELED/PASS** | lid close on AC with "no auto sleep on display off": Display off 21:54:47 / on 21:55:10, NO Entering Sleep; daemon PID 75517 continuous (uptime cross-window), runs/daemon NOT interrupted; Computer control fail-closed via the co-occurring screen lock. Blockers separated: user AC policy (idle only) vs bun agent `PreventUserIdleSystemSleep` assertions (idle only) — neither blocks explicit Apple-menu sleep. [Record](phase0c-display-off-awake-20261003.json) |
| **SYSTEM_SLEEP** | **PASS** | `willSleep` @14:24:24.914 + `screenIsLocked` + `screenIsUnlocked` + `didWake` @14:24:54.925 (NSWorkspace/DistributedNC); pmset cross-evidence (SRPrevSleep, kernel sleep-delay acks); P0 discarded; daemon survived sleep (PID 75517 uptime 5791797ms continuous); guard safe=true; fresh F1 @14:29:40 (capture ok, after didWake). [Record](phase0c-sleep-wake-20261003.json) |
| **FAST_USER_SWITCH** | **BLOCKED_NO_SECOND_USER** | only one real local user (tangxingpeng, UID 501); per protocol no test user created, no waiver. The `on_console=false` → fastUserSwitch fail-closed model path is exercised in every window (replay_blocked=true), but a real FUS transition was not executed. [Record](phase0c-fast-user-switch-20261003.json) |

### Cross-cutting invariants / 跨场景不变量

- **Pending action replay: PASS** — every window: `queue_replay_blocked=true`, fixture discarded with suspension reasons (stale frame / epoch / suspended).
- **Stale frame invalidation: PASS** — F0 rejected after each transition; a post-transition fresh frame (digest ≠ pre-transition, timestamp after return) was required and obtained in every executed scenario.
- **Controller epoch: PASS** — every suspension advanced the epoch and set `requires_human_resume`; no silent auto-resume.
- **Geometry revalidation: PASS** — every suspension transition increments `geometry_generation`; post-transition capture re-enumerates the display (1800×1169 re-confirmed each time).
- **DAEMON_COMPUTER_TCC_MUST_BE_NONE: PASS** throughout (guard safe=true; note the guard also proved itself earlier in the protected-roots gate).
- F2: CLOSED (no regression); Protected Roots: SUPPORTED (no regression).

### Incidents / 事件

- App Screen grant loss before scenarios (TCC record disappeared while the user cleaned daemon entries; user re-granted CodeBridge.app) — recorded, not a code bug.
- Two empty lock windows before the user actually locked (timing), and one empty sleep window before the user actually slept — retained as raw evidence, not instrumentation failures.
- First "sleep" attempt was DISPLAY_OFF_AWAKE, not SYSTEM_SLEEP — per user environment fact (AC + no-auto-sleep), correctly separated into its own state with both idle-sleep blockers (user policy and agent assertions) individually identified. Neither blocks explicit Apple-menu sleep, which succeeded.
