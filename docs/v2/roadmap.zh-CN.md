# CodeBridge V2 开发路线

状态：**实现路线基线**

架构是 Bridge-first，开发路线是 Computer-first。

这里刻意把“短暂的架构/上游基线准备”与“功能优先级”分开。**Computer Use 是 V2 第一优先级的新功能。**

## Phase 0 — Architecture Freeze 与上游基线

目标：先固定长期边界，再写新功能。

- [x] V2 Architecture。
- [x] Provider Contract。
- [x] Migration Strategy。
- [ ] 审计当前 CodexBridge 上游目录、进程和扩展点。
- [ ] 确认复用、License、NOTICE 义务。
- [ ] 原样构建 macOS 上游 App。
- [ ] Secure MCP Tunnel Smoke。
- [ ] Native Workspace / Shell / Docker Smoke。
- [ ] 定义 Native Service ↔ Go Runtime 的版本化 IPC。
- [ ] 定义 Project / Session / Run / Event / Artifact 的 ID 与 Schema Version。

完成标准：

- CodeBridge 可以薄复用上游，而不是大面积侵入修改。
- macOS Native Process 能收到 Bridge 请求并执行真实宿主命令。

## Phase 1 — Computer Use MVP（**最高功能优先级**）

目标：ChatGPT 能安全观察和操作真实 Mac。

### Computer Foundation

- [ ] MacComputerProvider。
- [ ] Screen Recording Permission。
- [ ] Accessibility Permission。
- [ ] Display Enumeration。
- [ ] 预留 Window/Application Target。
- [ ] Retina / Logical Coordinate。
- [ ] Frame ID。
- [ ] Display Change Invalidating。

### Observation

- [ ] `computer_start`
- [ ] `computer_observe`
- [ ] ScreenCaptureKit 静态截图
- [ ] 图片大小限制
- [ ] Permission Gate
- [ ] Unchanged Frame 明确语义

### Input

- [ ] move
- [ ] click
- [ ] double_click
- [ ] drag
- [ ] scroll
- [ ] type
- [ ] keypress
- [ ] wait
- [ ] Batch 首错即停
- [ ] controller_epoch 校验

### Lifecycle

- [ ] `computer_status`
- [ ] `computer_stop`
- [ ] Lease / Expiry
- [ ] Local Approval
- [ ] Metadata-only Event

### 必须通过的真实 E2E

- [ ] ChatGPT → Secure MCP Tunnel → CodeBridge → Observe Mac
- [ ] Act → Observe
- [ ] Permission Denied
- [ ] Resolution Change
- [ ] Stale Controller Epoch Reject

完成标准：

> 不依赖 Manager、Docker Client、公网 MCP 基础设施，ChatGPT 能在真实 Mac 上观察、点击、输入、滚动。

## Phase 2 — Live Computer + Human Takeover

目标：用户可以实时观看并安全接管。

- [ ] MediaProvider 基线。
- [ ] WebRTCMediaProvider。
- [ ] Short-lived Signaling Grant。
- [ ] STUN / Direct。
- [ ] TURN Fallback。
- [ ] Adaptive FPS / Resolution / Bitrate。
- [ ] ChatGPT Computer Widget。
- [ ] Pause。
- [ ] Take Over。
- [ ] Input Barrier。
- [ ] controller_epoch +1。
- [ ] Human Control。
- [ ] Resume Agent。
- [ ] Resume 后强制 Fresh Observe。
- [ ] Human Link 丢失后 Controller = none，禁止自动归还 Agent。

完成标准：

> Human 与 Agent 永远不能同时注入输入；Preview 故障不能破坏模型控制。

## Phase 3 — Runtime V2 迁移

目标：把当前已经有价值的 Runtime 基础迁入 Native 架构。

复用/迁移当前 Go：

- [ ] RunStore。
- [ ] Ordered RunEvent Journal。
- [ ] Subscription。
- [ ] RunManager。
- [ ] OMP Session Adapter。
- [ ] Codex Session Adapter。
- [ ] OpenCode Adapter。
- [ ] Recovery / Replay。
- [ ] Terminal Result。

统一到 V2 模型：

- [ ] Project。
- [ ] Session。
- [ ] Run。
- [ ] Turn。
- [ ] RuntimeEvent。
- [ ] Artifact。
- [ ] AgentContext。
- [ ] ComputerContext。
- [ ] CloudContext。

新增：

- [ ] Runtime UI。
- [ ] Conversation / Activity / Artifacts / Computer / Raw。
- [ ] Async Completion Notification。
- [ ] Provider Health。

完成标准：

> 长任务生命周期与 ChatGPT/Tunnel/UI 解耦，并能基于本地 Durable Journal 恢复。

## Phase 4 — OMP 与 Cross-provider Orchestration

目标：CodeBridge 从 Agent Gateway 升级为 Provider-independent Local Agent Runtime。

- [ ] Orchestrator Interface。
- [ ] Project Main Agent。
- [ ] Role Model。
- [ ] planner/coder/reviewer/operator。
- [ ] Delegation。
- [ ] TaskGraph。
- [ ] ReviewLoop。
- [ ] ModelPolicy。
- [ ] BudgetPolicy。
- [ ] Cross-provider Event Correlation。
- [ ] OMP-first 实现。
- [ ] Codex/OpenCode 保持独立 Provider。
- [ ] OpenAI Multi-agent 优先通过 Provider 接入，不重复造协议。

完成标准：

> 不修改 Bridge Core 就能把不同 Role 映射到不同 Provider / Model。

## Phase 5 — Local ↔ Cloud Handoff

目标：电脑睡眠时可接力，但 CodeBridge 不变成 Cloud Platform。

### Contract

- [ ] WorkspaceSnapshot。
- [ ] SnapshotProvider。
- [ ] CloudProvider。
- [ ] EnvironmentManifest。
- [ ] RunCheckpoint。
- [ ] Artifact Return。

### Incremental Ready

- [ ] Base Revision。
- [ ] Dirty Patch。
- [ ] Untracked Manifest。
- [ ] Environment Manifest。
- [ ] 笔记本在线期间持续 Ready Checkpoint。

### 第一 Cloud Provider

- [ ] 评估 Codex Cloud Handoff。
- [ ] Start Cloud Continuation。
- [ ] Fetch Diff / Artifact。
- [ ] Return Local。
- [ ] Conflict 显式处理，禁止静默覆盖本地。

后续 Provider：

- OpenAI Hosted；
- Self-hosted；
- User VPS。

完成标准：

> Run 可以显式切到一个 Cloud Provider，并安全返回 Diff / Artifact。

## Phase 6 — Additional Platforms

- [ ] Windows Native Bridge。
- [ ] Windows ComputerProvider。
- [ ] Linux Native Bridge。
- [ ] Linux X11/Wayland ComputerProvider。
- [ ] Platform-specific Permission。

## Phase 7 — Optional Sandbox

- [ ] Sandbox Execution Policy / Provider。
- [ ] Docker / VM Isolation。
- [ ] 明确与 Native Host 的 Capability 差异。
- [ ] 永远不把 Sandbox 重新设成个人开发者默认。

## Phase 8 — Fleet / Enterprise

只有真实需求出现以后再做：

- [ ] FleetControlPlane。
- [ ] Inventory。
- [ ] Central Policy。
- [ ] Version Management。
- [ ] Health。
- [ ] Audit Metadata。

Fleet 必须保持可选，不能进入个人版默认请求主链路。

## 明确停止作为主线投入的工作

V2 Critical Path 不继续投入：

- Public Manager MCP Ingress；
- Mandatory VPS；
- Caddy/Nginx MCP Routing；
- 自建 OAuth Ingress；
- Docker Client 作为 Mac 默认环境；
- Manager → Client gRPC 作为个人执行路径；
- 重复实现 CodexBridge 已成熟的 Desktop / Updater / Credential；
- 仅为了 Adapter 数量竞争而增加 Agent Adapter。

## 优先级冲突规则

1. Architecture / Security Correctness；
2. Computer Use；
3. Native Bridge Reliability；
4. Runtime Durability；
5. Orchestration；
6. Cloud Handoff；
7. Additional Platforms；
8. Fleet。
