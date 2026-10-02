# CodeBridge V2 开发路线

状态：**实现路线 —— 由 2026-10-02 架构评审修订**

架构是 Bridge-first；开发路线是 Computer-first。**Computer Use 是 V2 的第一个重大功能。**

每个阶段只构建它所需的长期骨架，不多做。Phase 1 构建 Computer Use 自身所依赖的 daemon、Policy 和 Journal 基础，因此 Computer Use 永远不会通过临时架构发布。

## Phase 0 — 基线、决策与技术验证

目标：在编写功能代码之前，关闭架构所依赖的证据缺口。

文档：

- [x] V2 架构基线。
- [x] Provider 接口。
- [x] 迁移方案。
- [x] 架构评审与修订（2026-10-02）。

上游（upstream）：

- [x] 定位 CodexBridge 上游：`Fanch-hui/codex-bridge`，公开 `win` 开发主线，Apache-2.0。
- [ ] 固定准确的上游 commit，并审计语言/技术栈/模块边界。
- [ ] 为每个复用模块记录 Apache-2.0 LICENSE / NOTICE 义务。
- [ ] 原样构建上游。
- [ ] 填写能力矩阵（[迁移方案](migration.zh-CN.md) §4.2），并为每个模块选择复用模式。
- [ ] 如果上游无法固定或不适用，记录该结论并在没有上游的情况下继续；没有任何 Phase 1 条目依赖上游。

技术验证（spike）：

- [ ] 入口（ingress）：`tunnel-client` → Streamable HTTP MCP over Unix-domain socket → `codebridged` → 在一个真实的 ChatGPT developer-mode 会话中应答的 `ping` 工具。
- [ ] 生命周期：带 `KeepAlive` 的 `SMAppService` LaunchAgent；退出 App 后 daemon 仍在运行；daemon 崩溃后自动重启。
- [ ] TCC 归属：确认 daemon 及其 harness 子进程**不会**被归属到 CodeBridge.app 的屏幕录制 / 辅助功能授权。
- [ ] Daemon 受保护 Root TCC：使用 `~/Documents`（以及等价受保护目录）中的 Project 实测按用户 LaunchAgent 能否在重建/更新后保持稳定的 Files-and-Folders 授权，并明确 v1 二选一结果：支持且给出 `host_permission_required` 指引，或明确不支持。
- [ ] 原生宿主：shell、git、Docker Desktop、SSH 和 kubectl 都能从 daemon 运行。
- [ ] 锁定状态行为：实测 screen lock、screen saver、wake、fast user switching 下的 ScreenCaptureKit / CGEvent 行为；确认排队输入不会在解锁后重放，并把结果写回 §5.5 故障矩阵。
- [ ] Widget 审批可行性（只作为 Phase 1 远程审批的门槛）：在支持的 ChatGPT surface 上完成 UI-only 审批工具往返，并通过 tool-result `_meta` 下发审批 token；不要求 WebRTC。
- [ ] widget 媒体可行性（仅作为 Phase 2 的门槛）：在 web、desktop 和 mobile 上的 ChatGPT widget 内使用 `RTCPeerConnection`；通过 UI-only 工具完成一次 signaling 往返。

定义：

- [ ] App 身份：bundle id、Team ID、开发构建的 Developer ID 签名（TCC 授权必须在重新构建后依然有效）。
- [ ] Host IPC v1 schema：`host`、`computer`、`approval`、`notify`。
- [ ] Store schema v1：projects、sessions、runs、provider_sessions、computer_sessions、events（`stream`、`seq`、`pos`）、artifacts、policy rules、grants、approvals、`schema_version`。
- [ ] Phase 1 工具的能力描述符。

完成标准：

- ChatGPT → Secure MCP Tunnel → `codebridged` 原生运行一条真实宿主命令，无需 Manager 或 Docker。
- App 退出后 daemon 依然存活；Host IPC 握手可用。
- 上游决策已被记录，即使结论是“未采用”。

## Phase 1 — Computer Use MVP（**最高功能优先级**）

目标：ChatGPT 能安全地观察和控制真实的 Mac。

### 1A. Daemon 基础（只做 Computer Use 需要的部分）

- [ ] `codebridged` 骨架与 Bridge 内核：入口（ingress）、调用方类别、能力注册表、分发、审计事件。
- [ ] Runtime store 与 Journal：Application Support 中的 V2 schema；状态 + 事件的事务性写入；按流的 `seq` 与全局 `pos`（复用 V1 的 journal 代码）。
- [ ] Policy v2 最小集：verbs + selectors、allow/deny/ask、`once` / `session` / `always` 作用域，审批由 CodeBridge.app 呈现，并提供需显式启用、仅用于审批的 `remote_human` widget 路径（仅 `once` / `session`）。没有模型可调用的 grant 工具。
- [ ] Project 注册时探测 TCC 保护 Root，缺权限时明确返回 `permission_denied: host_permission_required`，而不是不透明的 I/O 失败。
- [ ] Session 解析。
- [ ] `tunnel-client` 托管。

### 1B. 原生 computer engine（CodeBridge.app）

- [ ] 菜单栏 App 骨架与 Host IPC client。
- [ ] 权限检测：屏幕录制、辅助功能、输入监控；在使用时重新检查。
- [ ] 显示器枚举与几何信息；`geometry_generation` 由显示器重配置、睡眠和锁屏驱动。
- [ ] 使用 ScreenCaptureKit 做静态截图，排除 CodeBridge 窗口；限制尺寸与字节大小。
- [ ] 帧（frame）标记：`frame_id`、`arbiter_instance`、`controller_epoch`、`geometry_generation`；图像空间 → 逻辑 → 全局坐标映射（Retina）。
- [ ] InputArbiter：controller 状态机、epoch、租约（lease）、每个宿主只有一个输入持有者。
- [ ] 单一串行化的 CGEvent 注入器，逐事件重新校验并释放已持有的输入。
- [ ] 本地物理输入抢占（listen-only event tap）；如果监听器无法运行，`computer.input` 不可用。
- [ ] 紧急停止：菜单栏与全局快捷键。
- [ ] App 作用域强制：指针事件逐事件重新命中测试，键盘事件逐 key 重新检查 focused/frontmost App；受保护界面（CodeBridge 窗口、OS 安全界面、已配置 App）在真正注入前重新校验。

### 1C. 工具与端到端

- [ ] `computer_start`、`computer_observe`、`computer_action`、`computer_status`、`computer_stop`。
- [ ] 观测结果以 MCP image content 加 frame metadata 的形式返回。
- [ ] Action 原语：move、click、double_click、drag、scroll、type、keypress、wait；有序批次在首个失败处停止；每个批次都引用 `frame_id`。
- [ ] Journal 中只记录 metadata 的 computer 事件。

必须通过的真实 E2E（真实 ChatGPT 通过 tunnel）：

- [ ] 在真实 Mac 上 observe → act → observe。
- [ ] permission denied 与 approval-required 路径；审批只能来自本地 UI 的显式操作，或需显式启用的 `remote_human` widget；模型不能审批。
- [ ] 远程启动的 session 不依赖 WebRTC，即可通过纯审批 widget 获得 `once` / `session` 授权。
- [ ] controller 变更后，陈旧 frame 被拒绝。
- [ ] 分辨率/排列变化使 frame 失效。
- [ ] 物理鼠标/键盘操作立即抢占 agent。
- [ ] 指针或键盘输入落到受保护界面、越权 App 或越权焦点时会被拒绝。
- [ ] screen lock / screen saver / fast user switching 的行为符合 Phase 0 决策，且返回后不会重放排队输入。
- [ ] App 退出和 daemon 重启后 `controller = none`。

完成标准：

> 一个真实的 ChatGPT 会话能够通过 Secure MCP Tunnel 在用户的 Mac 上观察、点击、输入、滚动和拖拽 —— 无需 Manager、Docker Client 或公网 MCP 基础设施 —— 并且上述每一条负面路径都 Fail Closed（失败即关闭）。

Phase 1 退出评审：对照第一方 ChatGPT Computer Use 重新检查定位（[总体架构](architecture.zh-CN.md) §1.1）。

## Phase 2 — 实时预览与远程接管

目标：用户可以从 ChatGPT widget 观看并安全地接管控制。

前置条件：Phase 0 的 widget 媒体技术验证（spike）通过；否则在开始之前先选定无状态 rendezvous 兜底方案。

- [ ] computer engine 中的预览能力（复用同一套捕获流水线）。
- [ ] 由 daemon 签发的查看/控制授权（grant），通过 tool-result `_meta` 下发，绑定到 ComputerSession 与 widget 实例。
- [ ] 通过 tunnel 的 UI-only signaling 工具。
- [ ] STUN；TURN 使用本地签发的限时凭证。
- [ ] 自适应分辨率 / FPS / 码率；widget 隐藏时暂停。
- [ ] ChatGPT Computer widget：Pause、Take over、Resume agent、Stop。
- [ ] 远程人工输入经 data channel 进入 arbiter（`controller = human(remote)`）。
- [ ] 复用 Phase 1 的 `remote_human` 审批通道；增加实时预览/控制后不得扩大它的 scope 或信任级别。
- [ ] 人工链路丢失 → `controller = none`，绝不回到 agent。
- [ ] 恢复（Resume）需要人工操作；旧 frame 因 epoch 而失效。

完成标准：

> 人工与 agent 永远不会并发注入输入，预览故障永远不会影响模型控制，并且不涉及 Manager 或有状态的公共服务。

## Phase 3 — V2 上的 Runtime 与 agent

目标：长时间的 agent 工作运行在 `codebridged` 之下，使用与 Computer Use 相同的 Session、Policy 和 Journal。

从 V1 迁移：

- [ ] RunManager 状态机、并发、取消、超时。
- [ ] 基于 V2 Journal 的订阅与游标重放。
- [ ] OMP、Codex 和 OpenCode 作为 AgentProvider（启动代码 + session adapter 合并）。
- [ ] 位于 Project 之下的敏感路径、patch 和检查点（checkpoint）文件工具。

新增：

- [ ] 从实时输出绑定 ProviderSession；在同一 ProviderSession 上以新 Run 的形式恢复。
- [ ] 进程身份与孤儿进程处理；按 provider 决定是否 reattach（D10）。
- [ ] 按 provider 设置 autonomy 模式；`agent.start` 按 autonomy 上限授予。
- [ ] `agent_start`、`agent_status`、`agent_result`、`agent_cancel`、`runs_list`；UI-only 的 `events_wait`。
- [ ] 完成投递：widget 长轮询 + follow-up 消息；本地通知。
- [ ] CodeBridge.app 中的 Runtime 视图：Conversation、Activity、Artifacts、Computer、Raw。
- [ ] Raw 事件的保留上限。
- [ ] 一次性只读导入 V1 的 `runs.db`。
- [ ] 评估用 Codex `app-server` 做审批路由（D7）。

完成标准：

> 长时间工作能够跨越 ChatGPT、tunnel、UI 和 daemon 重启继续；状态可以从本地 Journal 恢复；模型从不轮询。

## Phase 4 — 编排（最小化）

目标：实现跨 provider 的工作，而不需要 CodeBridge 重新实现 multi-agent 协议。

- [ ] Run 树：`parent_run_id`、`role`；由 `agent_internal` 调用方创建的子节点自动建立关联。
- [ ] 沿整棵树进行权限衰减（attenuation）与预算继承。
- [ ] ModelPolicy：role → provider / model / autonomy。
- [ ] BudgetPolicy：wall-clock、token 与成本上限。
- [ ] Journal 中的跨 provider 关联。
- [ ] OMP-first 的 orchestrator，通过 CodeBridge 工具工作。
- [ ] 只有在有证据时才决定采用 CodeBridge 原生的 TaskGraph / ReviewLoop（D4）。

完成标准：

> 一个项目能够把 planner / coder / reviewer / operator 角色路由到不同的 provider 和 model，而无需修改 Bridge 内核。

## Phase 5 — 本地 ↔ 云端接力

目标：笔记本睡眠期间工作仍能继续，同时不让 CodeBridge 变成云平台。

- [ ] 基于 Git 的检查点（checkpoint），存放在私有 ref 上，在设备在线时增量准备。
- [ ] 未跟踪文件白名单；排除敏感路径。
- [ ] 从第一个真实集成出发定义 CloudProvider（D1）。
- [ ] Codex Cloud 路径：推送 checkpoint 分支 → `codex cloud exec` → 查看状态 → 把 `codex apply` 得到的 diff 作为 artifact。
- [ ] 接续链：新的 Run `kind = cloud`、`continues_run_id`、单一写入者。
- [ ] 显式地在本地应用到新的 branch/worktree；冲突必须暴露出来，绝不静默解决。

完成标准：

> 一个 Run 接力到一个真实的云 provider，并返回一个 diff，用户可以在本地应用它而不覆盖本地改动。

## Phase 6 — 其他平台

- [ ] Windows 和 Linux 上的 `codebridged`。
- [ ] Windows ComputerProvider。
- [ ] Linux X11/Wayland ComputerProvider。
- [ ] 把各平台的权限模型映射到同一套 verbs。

## Phase 7 — 可选沙箱执行

- [ ] 沙箱作为 agent 和 shell 工具的一项 autonomy/环境能力。
- [ ] Docker / VM 隔离，并明确与原生宿主的 capability 差异。
- [ ] 永远不作为个人开发者的默认环境。

## Phase 8 — Fleet（仅在需求得到证实后）

- [ ] 可选的出站 fleet connector。
- [ ] Inventory、健康状态、版本 metadata。
- [ ] 只能收紧权限的托管 policy 层。
- [ ] 永远不在请求数据路径上。

## 明确停止投入的主线工作

- 公网 Manager MCP 入口（ingress）；
- 强制 VPS；
- Caddy/Nginx MCP 路由；
- 为个人路径自建 OAuth 入口（ingress）；
- 把 Docker Client 作为 Mac 默认执行环境；
- 把 Manager→Client gRPC 路由作为个人执行路径；
- 模型可调用的权限授予；
- 重复实现已固定的上游已经提供的成熟 desktop/updater/credential 功能；
- 仅仅为了增加 adapter 数量而添加 agent adapter。

## 优先级规则

当优先级冲突时：

1. 正确性与安全；
2. [总体架构](architecture.zh-CN.md) §19 中边界的长期可维护性；
3. 上游（upstream）可同步性；
4. Computer Use；
5. Runtime 持久性；
6. 编排；
7. 云端接力；
8. 其他平台；
9. Fleet。
