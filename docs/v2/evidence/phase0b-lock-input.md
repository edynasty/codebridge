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
