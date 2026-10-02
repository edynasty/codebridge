# Phase 0B — Controlled TCC matrix / 受控 TCC 矩阵

Result: **SIGNED_TCC_BLOCKED_EXTERNAL**. **F2 NOT CLOSED.** No Phase 0B ScreenCapture, AX or listen-tap permission experiment ran. UNTESTED is not DENIED.

结果：外部真实签名前提缺失；本轮没有进行上述权限实验，不将 UNTESTED 填成 DENIED。

## Historical finding remains / 保留历史发现

Phase 0 daemon PID 72321 (parent 1) spawned harness 77878; actual 1800×1169 capture succeeded. TCC request 77878.1 identified responsible/authorization subject `a.out`, daemon path `bin/codebridged`, authValue 2/authReason 4. This is a real daemon-attributed authorization finding. It does **not** establish original App-grant causality, signed inheritance, or impossibility of the frozen topology. Original grant history is unresolved. Preserve [exact attribution](phase0-tcc-attribution.txt) and [original analysis](phase0-tcc.md).

历史截图及 daemon 归属事实不撤销；它不证明原始 App grant 的因果关系、真实签名继承或拓扑不可能。历史授权来源不明，不能强行计为 PASS。

## Matrix / 矩阵

| Context | Real execution / 实际执行要求 | ScreenCapture | AX query | Listen-only tap creation/delivery | Files & Folders |
| --- | --- | --- | --- | --- | --- |
| A signed App in-process | `--phase0b-probe permissions --roots "" --output ...`; separate `input-monitor` observation | UNTESTED | UNTESTED | UNTESTED | separate files-folders run, UNTESTED |
| B App child | signed bundled probe via App `--phase0-probe`, not A | UNTESTED | UNTESTED | UNTESTED | UNTESTED |
| C SMAppService daemon | API must execute in daemon PID; report daemon identity and TCC subject | UNTESTED | UNTESTED | UNTESTED | UNTESTED |
| D daemon harness | existing `host.tcc_probe` child, separately signed probe | UNTESTED | UNTESTED | UNTESTED | UNTESTED |
| E shell control | directly launched signed probe; record Terminal/shell responsibility | UNTESTED | UNTESTED | UNTESTED | UNTESTED |

The shared native probe now runs inside the App for A; identity-only smoke verified App executable/PID, not an OS grant. A/B/D/E instrumentation does not create C direct-native-call evidence. Current Go daemon diagnostics spawn a harness; those calls are **D**, even when TCC names daemon as the responsible subject. Direct C native API instrumentation is an acceptance prerequisite; do not relabel child execution as C.

已准备 A 同进程入口，身份 smoke 证明实际 App executable/PID，但不证明权限。现有 daemon 探针会创建 harness，因此属于 D；即使 TCC responsible 指向 daemon，也不能当成 C 的同进程 API 证据。C 直接 native API instrumentation 仍是后续验收的前置工作，未伪造为已完成。

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
