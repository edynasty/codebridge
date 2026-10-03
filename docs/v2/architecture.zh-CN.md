# CodeBridge V2 总体架构

状态：**ARCHITECTURE FROZEN —— 2026-10-02**

代码基线：`d251a3c Build durable runtime and harness session foundation`。文档基线：`8bcc5ba Define CodeBridge V2 architecture and migration plan`。

对于 V2，本文档取代 [`docs/runtime.md`](../runtime.md) 与 [`docs/computer-use.md`](../computer-use.md) 中相冲突的表述。这些文件仍是 V1 的设计记录；特别是其中的 Manager/gRPC 控制面以及由 Manager 持有的信令，都不属于 V2。

## 1. 产品定义

CodeBridge 把 AI 客户端连接到用户真实的开发环境。它既是一个 **Bridge**，也是一个 **Persistent Runtime**，还是一个 **Computer 控制层**。

三个地位相同的一级核心域：

- **Bridge** —— 请求如何进入、谁在调用以及允许调用什么：入口（ingress）、调用方身份、能力注册表、项目授权、策略/审批与密钥。
- **Runtime** —— 对工作的持久所有权：Session、Run、ProviderSession、事件 Journal、Artifact、恢复与通知。
- **Computer** —— 作为受控资源的真实桌面：观察、输入、独占控制、实时预览与人工接管。

> **Bridge-first architecture，Computer-first roadmap。**

Bridge 是架构的长期中枢。Computer Use 是当前最高的功能优先级。Runtime 为 agent、computer session 与未来的云端接续提供可靠的生命周期。

CodeBridge V2 **不是**：

- **一个 Computer 应用** —— 原生 macOS 应用只是宿主适配层；权威仍在 Bridge/Runtime daemon 中；
- **一个 CodexBridge 插件** —— 上游（upstream）代码在 CodeBridge 自有的适配层之后被消费；CodeBridge 从不把自己的领域注册进上游的工具注册表或会话模型；
- **一个云平台** —— 云是接续目标，而非权威记录系统；
- **一个 fleet 管理器** —— 个人路径上没有任何强制性的中心服务。

### 1.1 相对于第一方 Computer Use 的定位

2026-10-02 观察到：ChatGPT 桌面应用为 macOS 和 Windows 提供了第一方 Computer Use 插件，并支持按应用审批（[Codex Computer Use docs](https://developers.openai.com/codex/computer-use)）。因此，“模型能点击我的 Mac”本身并不构成差异化。

CodeBridge Computer Use 的正当性只来自第一方功能没有同时具备的性质：

- 可以从任意 ChatGPT 界面经由 Secure MCP Tunnel 触达这台机器，而不只是从运行在同一台机器上的桌面应用；
- 与文件、shell 和本地 agent 共用同一套 Session / Policy / Journal；
- 一份可被非 OpenAI 的 agent provider 使用的 ComputerSession 契约；
- 在 ChatGPT widget 内提供实时预览与人工接管。

Phase 1 的退出评审必须重新核查这一定位。

## 2. 设计目标

V2 必须做到：

- 默认运行在原生宿主之上；
- 对外暴露真实的开发环境，而不是 Docker 中的近似环境；
- 让面向 AI 的接口不依赖 OMP、Codex、OpenCode、macOS 以及任何单一 tunnel；
- 在 ChatGPT、tunnel 与 UI 断开期间保持本地工作存活，且不需要模型轮询；
- 让 Computer Use 与其他所有能力一样，通过同一套 Bridge、Policy 和 Journal 交付；
- 保证当人类控制或触碰机器时绝不注入 agent 输入；
- 为云端接续留下正确的接缝，但不绑定 Codex Cloud；
- 在能省力的地方复用 CodexBridge 的实现，但不让它拥有 CodeBridge 的领域；
- 为 Windows、Linux、更多 AI 客户端以及可选的未来 fleet 留出空间。

## 3. 证据基础

评审是拿代码和外部事实来检验设计的，而不只是拿 V2 文档本身。

### 3.1 在本仓库中已验证（`d251a3c`）

- `internal/agentops` 里有一个可用的 `RunManager`（状态机 `queued → running → completed | failed | cancelled | timeout | interrupted`、并发上限、取消、超时）、一个 SQLite `runStore`，以及按 Run 组织的 `RunEvent` journal，带 `SubscribeEvents` / `EventsAfter`。重启时，非终态 Run 会变为 `interrupted`；没有 reattach 或 resume。
- `agentops` 只导入 `internal/protocol` 和 `modernc.org/sqlite`。gRPC/Manager 的耦合只存在于 `cmd/client`（`run_event` / `run_heads` / `run_replay`）和 `internal/manager`（`RunEventBroker`、registry、OAuth）中。
- Harness 以 CLI 子进程方式启动，带各自的自动审批标志（`omp --auto-approve`、`codex exec --full-auto`、`opencode run --auto`）。
- OMP、Codex 和 OpenCode 的 `HarnessSessionAdapter` 实现已经存在并有测试，但**没有接入** Run；Run 不记录 provider 原生的 session id。
- 代码中不存在 Project、Session、Turn 或 Artifact 实体。
- Run 数据库位于 `os.UserCacheDir()`（macOS 上为 `~/Library/Caches/codebridge`）之下，这是一个可能被操作系统清理的位置。
- Run 行与 run event 是分两条语句写入的，而不是一个事务。
- V1 的 `permission_grant` 是一个模型可调用的 MCP 工具，被声明为只读，却能持久化 `always` 允许规则。模型可以批准自己的权限请求。
- Computer Use 没有代码，只有设计文档。
- CodexBridge 在本仓库中没有代码或 vendored 副本。目标上游现已从外部明确定位为 `Fanch-hui/codex-bridge`（默认开发主线 `win`，Apache-2.0）；但 CodeBridge 还没有固定具体 commit，也没有在本仓库中完成上游原样构建验证。

#### 3.1.1 Phase 0 实施更新

上面的基线保留为 `d251a3c` 时的 V1 记录。Phase 0 新增独立 Go daemon、最小 Swift App/probe、语言中立的 Host IPC/能力/Store 契约，以及 Application Support 下的 SQLite WAL Store。V1 保持不变。[实施收尾](evidence/phase0-closeout.zh-CN.md) 记录 **PHASE_0_BLOCKED**：真实签名下的 TCC 隔离、保护 Root 授权持久性与真实 ChatGPT/Tunnel 审批证据尚未建立。

### 3.2 外部验证（2026-10-02）

- Secure MCP Tunnel 是 OpenAI 的 [`tunnel-client`](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)：它向 OpenAI 发起出站 HTTPS long-poll，并把 MCP JSON-RPC 转发给本地 MCP server，传输方式为 **stdio 或 HTTP**；支持流式结果。它服务的是私有 / 开发者模式连接，而不是公开的插件分发。
- Apps SDK（[reference](https://developers.openai.com/plugins/reference)）：tool-result `_meta` 只投递给 widget，对模型隐藏；`_meta.ui.visibility` 可以让某个工具只能从 UI 调用；widget 通过 `window.openai.callTool` 调用工具；widget CSP 的 `connectDomains` 管控 fetch/XHR。
- Codex Cloud（[docs](https://developers.openai.com/codex/ide/cloud-tasks)）在由 GitHub 仓库构建的已发布环境中运行任务。CLI 提供 `codex cloud exec`、`codex cloud list` 和 `codex apply`（通过 `git apply` 应用云端 diff，冲突时报错失败）。完全可脚本化的环境/任务生命周期仍是一个未决请求（[openai/codex#24777](https://github.com/openai/codex/issues/24777)）。
- 第一方 ChatGPT Computer Use 已存在于 macOS/Windows 桌面应用中（§1.1）。
- 目标 CodexBridge 上游是 `Fanch-hui/codex-bridge`。Phase 0 已对 `win` / `v1.3.4`、commit `7844bb608a9a4e96ed09c084589b7825db77aa3e` 的实际源码、LICENSE/NOTICE、模块边界、原样构建和 smoke 做审计；[capability/reuse matrix](evidence/phase0-upstream.md) 明确记录失败与限制，不把文档描述当成实现证明。

### 3.3 证据缺口

| 缺口 | 阻塞 | 关闭方式 |
| --- | --- | --- |
| CodexBridge 上游 commit 与模块边界 | 具体上游复用 | **CLOSED**：[Phase 0 源码/构建审计与 capability/reuse matrix](evidence/phase0-upstream.md)；未导入上游源码或类型 |
| ChatGPT widget 沙箱（web、桌面、移动端）内的 WebRTC 及其 CSP 交互 | Phase 2 | Phase 0 技术验证（spike） |
| LaunchAgent daemon 及其 harness 子进程的 TCC 归属；它们**不得**继承 CodeBridge.app 的授权（grant） | Phase 1 安全 | Phase 0 技术验证（spike）。如果归属发生泄漏，则在发布任何 `computer.input` 之前，让 daemon 以独立的签名身份发布，和/或以声明免责（disclaimed responsibility）的方式启动 harness |
| `tunnel-client` 在 Streamable HTTP + Unix-domain socket MCP 目标模式下的受监管生命周期与重启行为 | Phase 1 入口（ingress） | Phase 0 技术验证（spike） |
| 以编程方式进行的 Codex Cloud 交接 | Phase 5 | 在 Phase 5 重新评估 |

本文档中没有任何架构规则依赖于未经验证的 CodexBridge 内部实现。

Phase 0B 对上面 disclaimer 备用方案的证据限定：**CONTINGENCY_NOT_PUBLICLY_SUPPORTED**（[公开 API 调研](evidence/phase0b-responsibility.zh-CN.md)）。Launch environment constraints 校验 responsible process，不会让进程声明免责；`ppid = 1` 不是 TCC 隔离证明。未实现私有 API/SPI 或 entitlement 绕过。F2 保持冻结且 **NOT CLOSED**。先实测独立、真实签名的 launchd 身份；只有可复现的签名隔离失败才需要最小架构修订，缺少证书本身不构成修订理由。[当前阻塞](evidence/phase0b-closeout.zh-CN.md)。

## 4. 领域分解

此前的 "CodeBridge Core" 拥有八项职责：能力路由、项目/工作区、session、权限/审批、身份/凭证、skills/工具、事件总线与扩展注册表。这是一个 god-core：它把无状态的请求流水线与各自拥有不变量的有状态领域混在了一起。V2 把它拆开。

```text
Bridge kernel (stateless request pipeline)
  ingress adapter -> caller identity -> capability registry
  -> policy decision -> dispatch -> result shaping + audit event
        |
        v
Domain services (own state and invariants)
  Project      Policy          Secret       Runtime              Computer broker
  roots,       rules,          refs ->      sessions, runs,      computer sessions,
  writable,    approvals,      Keychain     provider sessions,   lease mirror,
  sensitive    grants,                      journal, artifacts,  engine link
  paths        caller ceilings              recovery
        |                                        |
        v                                        v
Provider ports
  AgentProvider (OMP / Codex / OpenCode)    ComputerProvider (native engine over Host IPC)
```

| 组件 | 拥有 | 不得 |
| --- | --- | --- |
| Bridge kernel | 请求信封、调用方解析、能力注册表、分发、审计事件发送 | 持有领域状态；按具体 provider 分支；自行决定策略 |
| Project | project id、已授权的根目录、可写标志、敏感路径规则 | 执行任何东西 |
| Policy | 规则、审批请求、授权（grant）、调用方上限 | 渲染 UI；接受源自模型的决策 |
| Secret | 密钥引用与 Keychain 访问 | 把密钥返回给远程调用方，或写入 Journal |
| Runtime | Session、Run、ProviderSession、Journal、Artifact 元数据、恢复、通知游标 | 把 provider 的会话内容当作 CodeBridge 生命周期事实 |
| Computer broker | ComputerSession 记录、policy 绑定、到 engine 的路由、controller 变更的 Journal 记录 | 抓取像素或注入输入 |
| Native host adapter (CodeBridge.app) | Computer engine（capture、input、InputArbiter）、审批呈现、通知、预览媒体、更新器 | 持有持久领域状态；决定策略 |

从 Core 概念中移除：

- **Event Bus** —— Runtime Journal 是唯一的事件骨架。进程内扇出是 Journal 的一个特性：先提交，再通知。瞬时信号（预览统计、打字进度）不是事件。
- **Extension Registry** —— Go provider 在 daemon 启动时注册；进程外 provider 通过 Host IPC 以能力握手方式接入。V2 没有动态插件加载。
- **Skills** —— 由 agent harness 拥有（OMP skills、Codex skills、`SKILL.md`）。在出现具体消费者之前，CodeBridge 不对 skills 做归一化。
- **Identity / Credential** —— 拆分为调用方身份（Bridge kernel）与密钥（Secret 服务）。

## 5. 进程拓扑

```text
ChatGPT (web / desktop / mobile)              local MCP clients (opt-in)
        |                                              |
OpenAI Secure MCP Tunnel (OpenAI-hosted)               |
        | outbound HTTPS long-poll                     |
tunnel-client (OpenAI binary, supervised by daemon)    |
        | Streamable HTTP over Unix-domain socket      |
        +----------------------------+                 |
                                     |                 | optional local adapter
                                     v                 v
                              Unix-domain MCP ingress
+-------v---------------------------------------------------------------+
| codebridged — Go, per-user LaunchAgent — AUTHORITY                     |
|   Bridge kernel | Project | Policy | Secret | Runtime | Computer broker |
|   AgentProviders -> harness subprocesses (omp / codex / opencode)      |
|   runtime.db (SQLite WAL) in Application Support                      |
+-------^---------------------------------------------------------------+
        | Host IPC v1 (Unix domain socket, versioned JSON-RPC + binary attachments)
+-------+---------------------------------------------------------------+
| CodeBridge.app — Swift, menu-bar login item — NATIVE HOST ADAPTER      |
|   Computer engine: ScreenCaptureKit, CGEvent, Accessibility, InputArbiter |
|   approval presenter, notifications, runtime views, updater           |
|   preview media (WebRTC, Phase 2)                                     |
|   upstream-derived modules behind CodeBridge-owned adapters           |
+-----------------------------------------------------------------------+
```

### 5.1 为什么是两个进程

**决策：** 两个 CodeBridge 进程 —— `codebridged`（权威）与 `CodeBridge.app`（原生宿主适配层）。

原因按优先级排序：

1. **生命周期。** Run 必须在 UI 退出、崩溃和更新后存活。用户随时可能退出应用；由 launchd 拥有的 daemon 满足“UI 重启不结束工作”。
2. **TCC 隔离。** 屏幕录制、辅助功能和输入监控是独立的 macOS 隐私边界，与应用/进程责任归属有关。Agent 会执行任意 shell 命令，因此启动 harness 的进程绝不能被归属到 CodeBridge.app 的 Computer 权限。把所有受 TCC 约束的操作保留在 CodeBridge.app 中，并由独立管理的 daemon 启动 harness，可以形成我们需要的权限边界；但在发布 `computer.input` 前，Phase 0 必须在支持的 macOS 版本上实测 TCC attribution（§3.3）。
3. **平台划分。** Go daemon 跨平台，并且已经承载了 Runtime；computer engine 天然是按操作系统区分的（macOS 上用 Swift）。

被否决的替代方案：

| 替代方案 | 否决原因 |
| --- | --- |
| 用 cgo 调用 ScreenCaptureKit/CGEvent 的单一 Go 进程 | 把 TCC 授权放进启动 agent shell 的那个进程（违反 2）；ScreenCaptureKit 的异步 API 从 Go 调用很别扭；原生 UI 仍然必不可少 |
| 单一 Swift 进程（重写 Runtime） | 丢弃可用的 Go runtime 与 harness 代码；使 Windows/Linux 回退 |
| 把 Go runtime 作为库嵌入 Swift 应用 | 把崩溃域与生命周期耦合在一起（违反 1 和 2） |
| 由 Swift 应用以子进程方式监管 Go（此前 V2 的表述） | 退出 UI 会结束 Run，并把 Agent 进程树与持有 TCC 权限的 App 耦合，使我们要求的权限隔离难以证明和长期维护（违反 1 和 2） |

已接受的代价：一套带版本的 Host IPC 以及第二条工具链。缓解措施：Host IPC 的面积很小，而且**它本身就是** ComputerProvider 契约加上 UI 服务（[Provider 接口](provider-contracts.zh-CN.md) §7）。

### 5.2 职责

- **codebridged**：为每个 CodeBridge 工具提供 MCP 入口（ingress）；Bridge 内核；Project、Policy、Secret、Runtime 与 Computer broker；AgentProvider 与 harness 子进程；`tunnel-client` 监管；持久存储；恢复；通知游标。
- **CodeBridge.app**：computer engine；审批呈现；通知；runtime 视图；预览媒体；更新器；向 launchd 注册 daemon；派生自上游的桌面模块。
- **不属于 CodeBridge 的代码**：`tunnel-client`（OpenAI）与 harness CLI（第三方）。

### 5.3 生命周期归属

- **launchd 拥有 `codebridged`，且安装在 CodeBridge.app 之外**（2026-10-03 修订，Phase 0C spike 已实证）：daemon 是独立安装、独立签名的按用户 LaunchAgent，路径物理上位于 App bundle 外。修订依据（实测，非理论风险）：bundle 内拓扑中，TCC 把 daemon 的 Computer 权限授权 subject 解析为 `com.codebridge.app`，daemon 因此能以 App 的授权真实截屏（[Phase 0C 证据](evidence/phase0c-independent-daemon-tcc.zh-CN.md)）；daemon 位于外部路径且由 launchd 持有时，TCC 为其分配自身路径的独立 subject，默认拒绝全部 Computer 服务（[spike](evidence/phase0c-independent-daemon-tcc.zh-CN.md)）。原 `SMAppService.agent` 生命周期验收保留为 **HISTORICAL_PASS_FOR_BUNDLED_TOPOLOGY**；外部拓扑需要重新验收生命周期。与 App 保持同一产品版本、同一发布事务；不再要求“同一物理 bundle”。App 自身的登录项注册仍可使用 `SMAppService`。
- **`codebridged` 监管 `tunnel-client`**，因为 tunnel 必须比 UI 活得更久。
- **CodeBridge.app 是一个登录项。** 退出它会移除 Computer 与审批 UI 能力（Fail Closed），但 Run 不受影响。
- **开发模式**：`codebridged` 可以在前台终端中运行，使用同一个存储与 IPC。

### 5.4 数据权威

| 数据 | 权威 |
| --- | --- |
| projects、sessions、runs、provider-session 绑定、computer-session 记录、events、artifact 元数据、policy 规则、授权（grant）、审批 | 排他地归 `codebridged` 存储（`runtime.db`） |
| 密钥 | Keychain；存储中只保存引用 |
| 当前谁可以注入输入 | CodeBridge.app 内的 InputArbiter；易失；任何重启后都以 `controller = none` 开始 |
| harness 会话的对话内容 | provider 原生存储（OMP JSONL、Codex rollouts、OpenCode DB）；CodeBridge 只保留 locator 与投影 |
| TCC 权限状态 | 操作系统；使用时查询，绝不缓存为真值 |

CodeBridge.app 只保存 UI 偏好设置。

### 5.5 故障与重启矩阵

| 事件 | Run | Computer | 恢复 |
| --- | --- | --- | --- |
| `codebridged` 崩溃 | harness 进程失去监管者；重启后非终态 Run 变为 `interrupted`；孤儿进程组通过记录下来的进程身份被找到并终止，或在 provider 支持时进行 reattach（Phase 3） | IPC 断开时 engine 放弃 agent 控制（`controller = none`） | launchd 重启 daemon；调用方从 Journal 游标处继续 |
| CodeBridge.app 崩溃或退出 | 不受影响 | 每个 ComputerSession 变为 `suspended`；controller 为 `none`；所有帧（frame）失效 | 应用重启后重新握手；agent 必须重新观察 |
| tunnel 或 `tunnel-client` 挂掉 | 不受影响 | 进行中的模型动作失败；agent 租约（lease）按 policy 过期 | 监管者重启 `tunnel-client`；调用方查询状态 |
| ChatGPT 关闭 | 不受影响 | 租约过期 → controller 为 `none` | — |
| 更新 | 更新器请求 daemon 做准备；只要还有活跃 Run 就推迟重启，除非用户强制重启（强制 → `interrupted`，Phase 3 可恢复） | 会话结束 | 只向前的存储迁移，并自动备份 |
| 睡眠 / 屏幕锁定 / screen saver / fast user switching | Run 行为跟随 OS / provider；Phase 0 记录任何 provider-specific 的中断行为 | 检测到状态切换时，Computer 必须进入 `suspended`、令 `controller = none`、使 frame/geometry 失效并丢弃排队输入；返回后禁止重放输入 | Phase 0 实测 ScreenCaptureKit / CGEvent 的真实行为；恢复前必须重新观察 |

### 5.6 版本偏差

- Daemon 与 App 使用同一产品版本和同一发布事务；daemon 作为独立签名组件安装在 App bundle 之外、由 launchd 独立拥有，以维持 Computer TCC responsibility boundary（2026-10-03 修订，见 §5.3）。
- Host IPC 握手携带协议 `major.minor` 与能力标志。主版本不匹配会禁用 Computer 与 UI 服务（Fail Closed）。次版本 N 与 N-1 可互操作，以覆盖更新窗口期。
- 存储携带 `schema_version`；迁移只向前，且迁移前先做备份；不支持降级。
- 在 V2 内部，MCP 工具只做增量变更；破坏性变更会得到一个新的工具名。

## 6. 入口（ingress）与调用方身份

每个请求都会变成一个与传输无关的信封：

```text
Request
  request_id
  caller        {class, principal, ingress, hints}
  capability    tool name
  args
  session_id?   project_id?
  idempotency_key?
```

| 调用方类别 | 来源 | 上限 |
| --- | --- | --- |
| `remote_ai` | Secure MCP Tunnel —— ChatGPT 模型或 widget | 本地配置的远程上限；模型永不审批；widget 只能通过需显式启用的 `remote_human` 通道、凭借仅经 `_meta` 下发的审批 token 进行审批（§13.2） |
| `local_ui` | 通过 Host IPC 的 CodeBridge.app；审批必须由用户在原生审批 UI 中显式操作 | 可以审批；v1 信任“已验证签名的 App + 显式 UI 手势”，不把它描述成生物识别级的人在场证明 |
| `local_mcp` | 经由 stdio shim 的本地 MCP 客户端 | 需要显式启用；默认禁用 `computer.*` |
| `agent_internal` | 由 CodeBridge 启动的 agent run 用 run 作用域的 token 回调 | 最多不超过父 Run 的授权（grant） |

规则：

- **没有未认证的 loopback TCP 监听。** 默认绑定是一个 stdio shim，连接到 `0700` 按用户目录下的 Unix domain socket，并做 peer-UID 校验。如果真的启用 loopback HTTP 绑定，则要求 bearer secret 以及 Host/Origin 校验（DNS rebinding）。
- **Tunnel 是远程认证边界。** 只把它关联到用户自己的 ChatGPT workspace 与 Platform organization。可以固定 `openai/subject` 作为纵深防御；它只是一个提示，不是认证。V1 内嵌的 OAuth 授权服务器不属于个人路径。
- **在传输层，widget 调用与模型调用无法区分。** 它们经由同一个 tunnel 到达。唯一的区分方式，是持有随 tool-result `_meta` 投递的授权（grant），而模型永远看不到它（§12）。
- **不做 TCC 的 confused deputy。** 同用户的本地进程本来就拥有用户的文件与 shell 能力，但没有授予 CodeBridge.app 的屏幕录制 / 辅助功能授权。本地入口（ingress）不得把这些授权发放出去：`computer.*` 只通过 daemon 监管的 tunnel 入口提供给 `remote_ai`（以每次启动的 secret 认证），以及提供给 `local_ui`；`local_mcp` 需要显式启用。这缩小了同用户风险，但并未消除它；残余风险已被记录在案。

## 7. 领域模型

```text
Project            authorization boundary for files, shell and agents
Session            durable working context; groups work; scope for "session" grants; never completes
 ├─ Run            one execution with a state machine and a terminal result
 ├─ ProviderSession harness-native conversation binding (OMP session, Codex thread, OpenCode session)
 └─ ComputerSession leased control of one computer target
Event              journal entry on a stream
Artifact           metadata for a durable output
```

### 7.1 实体

以下为概念性字段；确切的 schema 在 Phase 0 定义。

**Project** —— `prj_…`；name；roots；可写标志；敏感路径策略。

**Session** —— `ses_…`；零个或一个 `project_id`；title；origin（调用方类别、外部会话（external conversation）提示）；status 为 `open | archived`。Session 没有运行态，也没有结果。它把 Run 与 ComputerSession 分组在一起，并且是 `session` 授权（grant）的作用域。

**Run** —— `run_…`（保留 V1 的 id 格式）；`session_id`；`project_id`；kind 为 `agent | cloud`；executor（provider、model、autonomy mode）；可选的 `role`、`parent_run_id`、`continues_run_id`、`provider_session_id`；status（V1 状态机不变）；运行期间的进程身份（pid、启动时间、进程组）；最终输出引用；错误类别；`last_seq`。

**ProviderSession** —— `psn_…`；provider；原生 id；locator（仅本地，绝不导出）；`project_id`；capabilities（resume、history）。每个 ProviderSession 至多有一个活跃 Run。

**ComputerSession** —— `cmp_…`；`session_id`；当由 agent run 驱动时的可选 `run_id`；target；state；controller `{kind: agent | human | none, holder, channel: local | remote}`；`controller_epoch`；租约（lease）`{holder, expires_at}`；已授予的 verbs 与应用选择器；`last_frame_id`；`geometry_generation`；capabilities。ComputerSession 是一个被租用的资源，而不是带结果的目标，因此它**不是** Run。

**Artifact** —— `art_…`；producer（run 或 computer session）；kind；reference（path、git ref、provider URI）；digest；size；sensitivity；retention class。只为持久输出创建：patch、commit、branch ref、报告、测试结果、云端结果，或用户显式保存的截图。观察帧（frame）不是 Artifact。

### 7.2 有意不做成实体的东西

- **Turn。** 各 harness 对 turn 的定义不同（Codex turns、OMP `turn_end`、一次性 `exec`），而 computer use 用的是 action batch。存储 Turn 只会重复 provider 原生状态。Turn 是事件上的可选 `turn_ref` 加上一个 UI 投影。
- **AgentContext / ComputerContext / CloudContext。** 由类型化记录取代：ProviderSession、ComputerSession 与 `kind = cloud` 的 Run。通用的 context 袋子终将变成什么都能塞的垃圾场。

### 7.3 命名规则

不加限定的 **Session** 一词永远指 CodeBridge 的 Session。另外两个永远带限定词：**ProviderSession** 与 **ComputerSession**。ChatGPT 的一次对话是一个**外部会话（external conversation）**。

### 7.4 共享契约，各自独立的状态机

此前的规则“agent、computer 与 cloud 共用一个生命周期模型”被取代：各领域共享 ID、Journal、policy 与 Artifact 契约，但它们**不**共享状态机。Run（目标 → 终态结果）与 ComputerSession（带 controller 的租用资源）是不同的东西。

### 7.5 关系

- 恢复一个中断或已完成的 agent run，会在同一个 `provider_session_id` 上创建一个带 `continues_run_id` 的新 Run。
- 编排产生的子节点是一个带 `parent_run_id` 的 Run。
- 云端接续是一个 `kind = cloud` 且带 `continues_run_id` 的 Run。
- Run 绝不在 executor 之间迁移。

### 7.6 Session 解析

显式给出的 `session_id` 必须属于调用方的 principal。否则 Bridge 使用外部会话（external conversation）提示加上 project 来寻找一个打开的 Session，或者创建一个。仅有 session id 本身绝不构成任何授权。

### 7.7 V1 兼容性

`agent_runs` 映射为 `kind = agent` 的 Run 加上新列；`agent_run_events` 映射为流 `run:<id>` 上的事件，并沿用它们现有的 `seq`。RunStatus 状态机以及 RunManager 的并发、取消与超时语义保持不变。

## 8. 事件 Journal

```text
RuntimeEvent
  pos           global monotonic journal position (local subscription cursor)
  stream        run:<id> | computer:<id> | session:<id> | system
  seq           contiguous per stream (gap detection, idempotent replay)
  at
  session_id    project_id?
  source        bridge | runtime | agent | computer | policy | system
  kind
  turn_ref?     correlation_id?
  payload       bounded, metadata first
  raw_ref?      local locator of a provider raw record
```

序号决策：

- **按流的 `seq`** 泛化了 V1 按 Run 的 `seq`；V1 事件保留原有编号；
- **全局 `pos`** 用一个游标（而不是每个 Run 一个）服务“X 之后的所有内容”这类订阅（UI、通知）；
- **没有按 session 的序号**：Session 长期存活并且并发运行工作；按 session 的计数器只会把不相关的写入者串行化，却带不来任何额外保证。

权威性与持久性：

- Journal 对 CodeBridge 的生命周期事实具有权威：Run 状态、审批与授权（grant）、controller 变更、Artifact 创建。
- 状态变更与其事件在**一个事务**中提交（V1 是分开写的）。
- Run 的终态转换与审批决策在被确认之前就已持久化。
- 实时投递只是提交之后的通知；订阅者通过游标恢复。
- 对话内容的权威仍然是 provider 原生历史。

绝不写入 Journal：像素、视频、密钥、完整文件内容、隐藏的模型推理。Computer 事件携带帧（frame）元数据（id、尺寸、digest、display、epoch），绝不携带图像。

保留策略：生命周期与 policy 事件予以保留；`harness.raw` 与啰嗦的 provider 事件受保留上限约束（V1 会永远保留它们）。

初始 kind：`run.*`、`agent.message`、`agent.tool.*`、`provider_session.bound`、`policy.approval.requested`、`policy.approval.decided`、`computer.session.*`、`computer.observed`、`computer.action`、`computer.controller.changed`、`artifact.created`、`host.health`。

## 9. Provider

本节为摘要；细节见 [Provider 接口](provider-contracts.zh-CN.md)。

- **现在冻结（Frozen）**（已有实现，或属于 Phase 1）：AgentProvider、ComputerProvider（Host IPC 的 `computer` 服务）、capability descriptor 与事件发射契约。
- **内部端口**，不是公开扩展点：ingress adapter、SecretStore、ApprovalPresenter、Notifier、TunnelSupervisor。
- **延后（Deferred），有意不定义**：CloudProvider、workspace 检查点（checkpoint）（原 SnapshotProvider）、独立的 MediaProvider、SkillProvider、ControlPlane/Fleet 与沙箱执行。只写下了对它们的约束条件。

依赖规则：

- Provider 只依赖它实现的端口、事件发射契约和错误模型。
- **Provider 绝不调用其他 provider。** 组合发生在领域服务中（由 Runtime 协调 checkpoint 与 cloud），或者通过 Bridge 请求发生（需要 computer 的 agent 以 `agent_internal` 身份调用 CodeBridge 工具）。
- Bridge 内核绝不导入具体的 provider。

## 10. Runtime 可靠性

- 一旦 Run 启动，它就归 `codebridged` 所有，而绝不归发起它的那个请求。
- ChatGPT、tunnel 与 UI 的断开不会结束 Run（§5.5）。
- 恢复：启动时，非终态 Run 变为 `interrupted`（沿用 V1 行为）。记录下来的进程身份用于定位孤儿 harness 进程组；除非 provider 支持 reattach，否则它们会被终止。Resume 会在同一个 ProviderSession 上创建一个新 Run。
- **无需模型轮询的通知。** 在 Secure MCP Tunnel 之上，ChatGPT 侧只会收到由它自己发起的请求所对应的数据。因此：
  - 模型永不轮询；
  - 可见的 widget 可以维持一个有界 long-poll（`events_wait(cursor, timeout)`，一个仅 UI 可用的工具），并在终态事件发生时发送后续消息；
  - CodeBridge.app 发送本地通知；
  - 存在状态工具，供用户显式检查和恢复。
- 不需要 Manager、SSE 中继或公开推送服务。
- 存储从 `~/Library/Caches` 迁移到 `~/Library/Application Support/CodeBridge/`。

## 11. Computer

### 11.1 组件

- **Computer broker**（daemon）：ComputerSession 记录、policy 绑定、MCP 工具、Journal 记录。
- **Computer engine**（CodeBridge.app）：ScreenCaptureKit 采集、CGEvent 注入、Accessibility 查询，以及 **InputArbiter**。engine 是唯一会投递输入事件的组件。

### 11.2 目标

| 目标 | 状态 |
| --- | --- |
| `display` | Phase 1 的输入与观察目标 |
| `window`、`application` | 目前用作观察过滤器与输入作用域；以后作为主要目标（窗口移动与遮挡会使窗口相对的输入在不做额外检查时变得不安全） |
| `browser` | 未来独立的 target/provider，带 `browser.*` 权限，因为 DOM 访问会暴露 cookie 与凭证 |
| `virtual` | 未来的隔离桌面或 VM |

### 11.3 帧（frame）与坐标

- 每一次观察都会产生一个**帧（frame）**：`frame_id`、display id、图像尺寸、display 逻辑尺寸、缩放因子、display 原点在全局坐标中的位置、采集时间、`arbiter_instance`、`controller_epoch`、`geometry_generation`、被排除的窗口。
- 模型坐标位于**所引用帧的图像空间**中。engine 负责把图像 → display 逻辑点 → 全局 CoreGraphics 坐标做映射。Retina 缩放与面向模型的降采样对模型不可见。
- 图像在尺寸与字节数上都有上限；降采样会记录在帧中。

### 11.4 Controller 状态机

ComputerSession 的状态：`starting → active ⇄ suspended → closed`，或 `failed`。

`controller ∈ {agent, human, none}`；`human` 携带 channel，为 `local` 或 `remote`；`holder` 标识持有控制权的 ComputerSession 与调用方。

- **每个宿主 GUI 登录会话只有一个输入持有者。** 键盘焦点是宿主全局的；多显示器不会产生相互独立的输入通道。仅观察的 session 可以并发运行。
- 每一次 controller 变更都会递增 `controller_epoch`，它在同一个 `arbiter_instance` 内单调递增。重启会创建一个新的 instance。
- **初始获取** `none → agent` 需要 `computer.input` 授权（grant）和一个租约（lease）。
- `agent → human`（接管）是立即的。
- `agent → none` 发生在暂停、租约过期、本地输入抢占、紧急停止、权限撤销、目标 display 变更或 IPC 丢失时。
- 当人工链路丢失时 `human → none` —— 绝不 `human → agent`。
- **人工保持（human hold）。** 任何由人发起的转换（接管、暂停、本地输入抢占、紧急停止）以及任何人工链路的丢失，都会把 session 置于人工保持（human hold）。要脱离人工保持转向 `agent`，需要人工显式 Resume；模型无法重新夺回控制权。其他转向 `none` 的转换（租约过期、display 变更）允许在授权（grant）仍然有效的前提下重新获取控制。
- IPC 丢失、应用重启或 daemon 重启会结束所有 session 的 agent 控制；工作只能在一个新打开或重新激活的 session 中、基于全新观察继续进行。

### 11.5 输入有效性 —— 为何仅靠 `controller_epoch` 还不够

只做 epoch 检查会漏掉：已经在执行中的动作、基于过时屏幕规划的输入、几何变化，以及在与注入不同的进程中所做的检查。因此每个 action batch 都会引用它规划所依据的 `frame_id`，并且 engine 仅在下列条件满足时才执行：

1. ComputerSession 处于 active、持有输入且 `controller = agent`；
2. 该帧的 `arbiter_instance` 与 `controller_epoch` 等于当前值 —— 这也会强制在任何 controller 变更（包括 Resume）之后做一次全新的观察；
3. 该帧的 `geometry_generation` 等于当前值（分辨率、排列或缩放变化都会使其失效）；
4. 该帧的年龄在 policy 上限之内；
5. 当前输入目标位于已授予的应用作用域内，且不是受保护界面（§11.7）：指针事件重新命中测试该坐标下的窗口/应用；键盘事件重新检查当前获得焦点/最前台的应用与焦点目标。

在单一串行化的 injector 上，检查 1–5 会在每次提交底层事件之前立即重做。因此指针命中测试和键盘焦点检查发生在注入时，而不是只在 batch 开始时做一次。batch 执行中途发生变化会中止剩余部分。中止或被抢占时，engine 会释放它所持有的每一个按键与鼠标键。batch 按顺序执行，并在第一次失败时停止。

macOS 仍可能在最后一次 OS 查询与 CGEvent 真正投递之间改变焦点或窗口层级。CodeBridge 无法在存在并发/对抗性窗口变化时证明事件一定落到最初预期的窗口；这是明确记录的残余竞态。只要能够观察到焦点、作用域或受保护界面发生变化，就必须立即中止。

校验与注入在同一进程内完成。如果在 daemon 中校验、在 app 中注入，就会留下一个跨进程的检查时刻/使用时刻（time-of-check/time-of-use）窗口。

### 11.6 人工/agent 互斥

不变量：**当人类控制机器或正在物理使用机器时，CodeBridge 绝不注入 agent 输入。**

- **本地输入抢占。** 一个只监听的事件 tap 检测并非由 engine 发起的硬件输入。当 `controller = agent` 时，它会在下一个 agent 事件之前抢占为 `none`（epoch++）。如果无法安装该监控（缺少权限），`computer.input` 不可用 —— Fail Closed。
- **紧急停止。** 菜单栏控制项与全局快捷键，由 arbiter 在进程内处理。
- **远程人工输入**（Phase 2）仅在 `controller = human(remote)` 时通过预览数据通道进入 engine。

### 11.7 受保护界面

采集会排除 CodeBridge 自己的窗口（审批提示、授权（grant））。输入被拒绝投递给 CodeBridge 自己的窗口、操作系统的安全界面（系统设置中的隐私面板、认证与安全代理、Keychain 访问），以及用户配置的应用（如密码管理器）。受保护界面与 App 作用域检查必须在每一次注入指针或键盘事件之前重新执行。agent 绝不能通过点击来批准自己的权限。

### 11.8 权限与隐私

- `computer.observe` —— 截图会离开本机发送给 AI provider；这一点会告知用户。
- `computer.input` —— 始终携带应用选择器。
- `computer.preview` —— Phase 2。

授权（grant）受 ComputerSession 租约（lease）约束。TCC 权限在 session 启动时以及每次使用时都会检查，因为它们随时可能被撤销。帧（frame）只存在于内存中，默认不持久化。

### 11.9 多显示器、分辨率与应用切换

- Phase 1 把 session 绑定到单个 display；落在它之外的输入会被拒绝。
- Display 重新配置会递增 `geometry_generation`；如果目标 display 消失，session 会被挂起。
- 在应用作用域内允许切换应用或窗口；会落到作用域之外的输入会被拒绝。
- 睡眠或屏幕锁定会挂起 session。

### 11.10 动作模型

**原语（已冻结）：** `move`、`click`、`double_click`、`drag`、`scroll`、`type`、`keypress`、`wait`。线上格式是带能力协商的 tagged union；未知 kind 会以 `unsupported` 被拒绝。`type` 与 `keypress` 在每一个实际发送的 key event 前都必须重新检查当前焦点/最前台 App 是否仍在授权作用域内；一旦焦点切到越权或受保护目标，立即中止剩余输入。

不属于 computer 动作：

- `script`（AppleScript、由 shell 驱动的 UI 自动化）属于 shell/自动化能力，归 `shell.execute` 类 policy 管，因为它会绕过帧（frame）与应用作用域检查。
- `browser_script` 属于未来的 browser target/provider，带 `browser.*` 权限。
- `workflow` 是 Runtime 关注的事（一个 Run 或编排），由原语组合而成。

未来的观察模式可能返回 accessibility tree（`observe.ax_tree` 能力）。

## 12. 数据面

- **模型面（MCP）：** 观察以 MCP image content 加帧元数据的形式呈现；action batch；状态。
- **人工面（WebRTC，Phase 2）：** 从同一采集流水线到 ChatGPT widget 的实时视频；远程人工输入经由 data channel。

规则：

- 实时视频绝不进入模型上下文；
- 模型控制绝不依赖 WebRTC；
- 媒体故障绝不改变 ComputerSession 状态，唯一例外是失去远程人工控制者时会把 `controller = none`；
- 预览默认绝不录制。

最小基础设施 —— **没有 Manager**：

- **信令：** 经由同一个 tunnel 的仅 UI 可用的 MCP 工具（`_meta.ui.visibility = ["app"]`）；携带已收集候选的 SDP。默认没有公开的信令服务。
- **授权（grant）：** daemon 签发绑定到某个 ComputerSession 的短时 view/control 授权，随 tool-result `_meta` 投递（仅 widget 可见，对模型隐藏）；在可用时绑定到 widget 实例（`openai/widgetSessionId`），停止时撤销。
- **STUN：** 公开或自行配置。
- **TURN：** 唯一必需的公开组件，用于直连 ICE 失败的网络。一个无状态中继；凭据在本地按查看者签发，使用有时限的 HMAC（TURN REST 风格）。它只能看到 DTLS-SRTP 密文。
- **回退方案：** 如果 widget WebRTC 或基于工具的信令被证明不可行（Phase 0 技术验证（spike）），就使用无状态的 rendezvous 中继 —— 一个以不可猜测的 grant id 为键的邮箱，没有账号，也没有 session 状态。它绝不能演变成一个 Manager。

## 13. 安全模型

### 13.1 权限

Permission = **verb + resource selector**。

- Verbs：`filesystem.read`、`filesystem.write`、`shell.execute`、`agent.start`、`agent.cancel`、`computer.observe`、`computer.input`、`computer.preview`、`git.push`、`cloud.upload`、`cloud.execute`、`network.connect`、`secret.read`。
- Selectors：project 与 path glob；命令前缀；app bundle id；host 与 port；secret id；provider 与 autonomy mode。
- Effects：`allow`、`deny`、`ask`。Deny 永远优先。未匹配即 `ask`。
- 授权（grant）作用域：`once`、`session`、`project`、`always`。Computer 授权还受租约（lease）约束；`project` 作用域不适用于 computer verbs。

### 13.2 审批通道

| 通道 | 允许的作用域 | 说明 |
| --- | --- | --- |
| `local_ui` | all | 最强；每一次决策都要求用户在 CodeBridge.app 中显式操作 |
| `remote_human`（widget，需显式启用） | `once`、`session` | 从 Phase 1 起用于纯审批流程；绝不 `project` / `always`；绝不 `secret.read` 或 `cloud.upload`；需要一个只在 `_meta` 中投递的审批 token |
| model | none | 没有任何模型可见的工具能创建或放宽授权（grant）；V1 的 `permission_grant` 不再沿用 |

**本地人工信任边界：** v1 把“代码签名已验证的 CodeBridge.app + 前台审批 UI 中的显式用户手势”视为 `local_ui`。v1 不要求每次审批都通过 LAContext / Touch ID。App 进程被攻陷仍是明确记录的残余风险；未来可对特定高影响授权增加更强的人在场认证，而不改变 Policy 模型。

**权限衰减（attenuation）：** 有效权限 = 本地 policy ∩ 调用方类别上限 ∩ 父 Run 的授权（grant）。远程调用方不能放宽本地权限；子 Run 不能超过其父级。

### 13.3 Harness 自主性

V1 的 harness 带着各自的自动审批标志运行，因此 `agent.start` 实际已经授权了 harness 的完全自主。V2 把这一点显式化：provider 声明 autonomy mode（例如只读、workspace-write、full host），Policy 按 autonomy 上限授予 `agent.start`。把 harness 原生的审批请求路由进 Policy 是后续的 provider 能力。

### 13.4 执行限制

明确说明，而非隐藏：

- 命令前缀规则是 UX 过滤器，不是沙箱；
- `network.connect` 只对 CodeBridge 发起的出站流量（上传、TURN、云端 API）生效，对原生宿主上的 agent 子进程流量不生效；
- 真正的隔离需要可选的沙箱执行（Phase 7）。

### 13.5 密钥、保留与 IPC

- 密钥存放在 Keychain 中；存储只保存引用。密钥绝不进入 Journal、MCP 结果或模型上下文，并且只注入到拥有它们的那一个 provider。
- 帧（frame）只存在于内存；预览绝不录制；原始 harness 事件受保留上限约束；每个 Artifact 都有 retention class。
- Host IPC 使用 `0700` 按用户目录下的 socket，校验 peer UID，验证声明为 app 角色的对端代码签名，并协商版本。
- 位于 macOS TCC 保护目录下的 Project Root 绝不默认可访问。Phase 0 必须验证 `codebridged` 能否在重建/更新后保持稳定的按用户 Files-and-Folders 授权。注册 Project 时必须探测 Root：缺少宿主权限时返回 `permission_denied`，reason 为 `host_permission_required`；如果无法为 daemon 建立稳定授权，则 v1 明确不支持这些受保护 Root，而不是等到运行时才表现为不透明的 I/O 错误。

### 13.6 Computer Fail Closed

出现以下任一情况时输入即停止：TCC 权限缺失或被撤销；IPC 丢失；daemon 重启；租约过期；arbiter 状态未知；本地输入监控失败；display 变更；命中受保护界面。

### 13.7 上游代码

派生自上游的代码在 CodeBridge.app 内运行，并携带其 TCC 授权。每一次上游同步都是一次安全评审。

## 14. 编排

**CodeBridge 拥有：** run 树的记账（`parent_run_id`、`role`）、沿整棵树进行权限衰减（attenuation）与预算约束、provider/model 路由（ModelPolicy）、跨 provider 关联、持久状态。

**Provider 拥有：** 它们内部的多 agent 协议（OMP subagents、Codex subagents、OpenAI Agents SDK handoffs）、提示与规划。

**最小抽象：** orchestrator 就是任何以 `agent_internal` 身份通过 CodeBridge 工具启动子 run 的 AgentProvider run；CodeBridge 自动把它们关联起来。ModelPolicy 把 role 映射到 (provider, model, autonomy)。BudgetPolicy 定义 wall-clock、token 与成本上限，由子级继承。

**延后（Deferred）：** CodeBridge 原生的 TaskGraph 与 ReviewLoop 引擎。只有当以 OMP 为先、通过工具进行编排被证明不够用时，才重新考虑。

## 15. Local ↔ Cloud 交接

- 一次交接会创建一个**新的 Run**（`kind = cloud`、`continues_run_id`）。Run 绝不迁移。
- **单一写入者：** 每条接续链上恰好只有一个活跃 run。
- **本地权威：** 对于本地文件，本地工作区是权威。云端结果以 Artifact（patch 或 branch）的形式返回。应用这些结果是一次显式的本地操作，落到新分支或 worktree 上并做冲突检测 —— 绝不静默地应用到脏工作树。
- 链的元数据留在本地 Journal 中；云端状态以事件镜像进来。
- **检查点（checkpoint）**基于 git：base commit 加上私有 ref（`refs/codebridge/checkpoints/…`）上的一个 WIP commit。它们在机器处于唤醒状态时，于 Run 里程碑和空闲点增量准备，因为睡眠通知留下的上传时间太少。未跟踪文件仅通过 allowlist 纳入；敏感路径被排除（复用 V1 的敏感路径规则）。
- 上传需要单独的权限（`git.push` / `cloud.upload`，并指定明确的目的地）。
- **Codex Cloud 的现实：** 环境在服务端由 GitHub 仓库配置；任务用 `codex cloud exec` 提交；diff 通过 `codex apply` 返回。CodeBridge 不得假设 SSH、VM 语义、任意快照上传或上传环境 manifest。因此一次 Codex Cloud 交接意味着：推送 checkpoint 分支 → 提交一个引用它的任务 → 把 diff 作为 Artifact 取回。
- 在集成第一个真实 provider 之前（Phase 5），CloudProvider 与 checkpoint 契约保持不定义。

## 16. 上游策略

CodexBridge 是实现来源，而不是拥有者。Phase 0 已固定 `7844bb608a9a4e96ed09c084589b7825db77aa3e`（`win`、`v1.3.4`），并记录逐模块复用决策（[审计](evidence/phase0-upstream.md)）；未导入上游源码。复用模式、适配层、补丁队列与同步流程在 [迁移方案](migration.zh-CN.md) §4 中定义。约束性规则：

- CodeBridge 拥有 MCP 接口面、Policy、Runtime 与 Computer 契约；上游代码绝不注册或路由 CodeBridge 工具。
- 上游代码由 CodeBridge 自有的适配层包裹；上游类型绝不进入 CodeBridge 领域代码或 Host IPC，CodeBridge 类型也绝不进入上游代码。
- CodeBridge.app 有自己的 bundle id 与签名身份；TCC 授权绝不与上游应用共享。

## 17. Fleet

- **没有 ControlPlaneProvider。** "LocalControlPlane" 就只是那个 daemon，而且不存在第二份实现。
- **Manager 不属于 V2。** V1 的 Manager/Client 在冻结的检查点（checkpoint）处保持可构建。
- 未来的 fleet 以可选出站连接器的形式从 `codebridged` 接入：向外发送清单、健康与版本元数据；向内接收受管 policy。受管 policy 只能收紧（deny 优先的分层）。Fleet 绝不位于请求数据路径上。
- 届时值得重新审视的 V1 代码：设备注册与轮换（`authstore`）、JWT 校验器（`oauthresource`）、审计格式、`RunEventBroker` 的重放逻辑以及 registry 背压。

## 18. 存储

- 位置：`~/Library/Application Support/CodeBridge/`（`runtime.db`、`artifacts/`）；日志在 `~/Library/Logs/CodeBridge/`。
- 可以持久化：project、session、run 与 provider-session 元数据；有序事件；provider locator；Artifact 元数据与显式保留的 payload；终态结果；policy 规则与授权（grant）；checkpoint 元数据；UI 索引。
- 默认绝不持久化：屏幕视频、截图、密钥、用于遥测的源码副本、隐藏的模型推理。

## 19. 决策登记

### 19.1 冻结（Frozen）

| ID | 决策 |
| --- | --- |
| F1 | `codebridged` 是唯一权威；CodeBridge.app 是不持有持久领域状态的宿主适配层 |
| F2 | 两个进程；launchd 拥有 daemon；只有 app 持有 TCC 授权；harness 绝不继承它们 |
| F3 | MCP 入口（ingress）终止于 daemon；`tunnel-client` 只是传输；没有未认证的 loopback TCP |
| F4 | 调用方类别、审批通道与权限衰减（attenuation）；没有任何模型可见的路径能创建或放宽授权（grant） |
| F5 | 实体：Project、Session、Run、ProviderSession、ComputerSession、Event、Artifact；Turn 不是实体 |
| F6 | Journal：按流连续的 `seq` + 全局 `pos`；状态变更与事件在同一事务中 |
| F7 | Computer：动作引用帧（frame）；arbiter 与 injector 在同一进程；每个宿主只有一个输入持有者；本地输入抢占；仅人工可恢复；受保护界面；Fail Closed |
| F8 | 原语动作集；`script` / `browser_script` / `workflow` 不是 computer 动作 |
| F9 | MCP/WebRTC 分离；没有 Manager；TURN 是唯一必需的公开组件 |
| F10 | AgentProvider、ComputerProvider、capability descriptor、事件发射与错误契约（v1） |
| F11 | 交接 = 新 Run + 接续关系；本地权威；显式应用 |
| F12 | 没有 ControlPlane 接口；未来的 fleet 只能收紧 |
| F13 | 上游位于 CodeBridge 自有的适配层之后；上游绝不导入 CodeBridge，CodeBridge 领域绝不导入上游类型 |

### 19.2 延后（Deferred）

| ID | 决策 | 触发条件 |
| --- | --- | --- |
| D1 | CloudProvider 与 checkpoint 契约 | 第一次真实的云端集成（Phase 5） |
| D2 | 独立的 MediaProvider | 除 computer preview 之外出现第二个媒体来源 |
| D3 | SkillProvider | 出现需要跨 harness 归一化 skills 的具体消费者 |
| D4 | CodeBridge 原生的 TaskGraph / ReviewLoop | 以 OMP 为先、通过工具进行编排被证明不够用 |
| D5 | 把 window / application / browser / virtual 作为主要输入目标 | display MVP 得到验证 |
| D6 | 独立的 Swift computer 辅助进程 | UI 崩溃对 computer 可靠性的损害达到可测量程度 |
| D7 | 把 harness 原生审批路由进 Policy | 按 provider 逐个推进，Phase 3 及以后 |
| D8 | 通过 tunnel 做 OAuth 以实现多用户访问 | 一个 tunnel 必须服务不止一个人 |
| D9 | 沙箱执行 | Phase 7 |
| D10 | 孤儿进程是 reattach 还是终止 | 按 provider 逐个推进，Phase 3 |
| D11 | 每个上游模块的复用模式 | 已由 [Phase 0 审计](evidence/phase0-upstream.md) 关闭；实际导入仍需遵循适配层与署名流程 |

## 20. 架构不变量

1. Bridge 内核与领域服务绝不依赖具体的 agent、操作系统、传输、媒体或云端 provider。
2. 每一次能力使用都经由 Bridge 内核与 Policy 进入；provider 绝不调用 provider。
3. `codebridged` 存储是 CodeBridge 状态的唯一持久权威。
4. 各领域共享 ID、Journal、policy 与 Artifact 契约；在语义不同的地方，它们保留各自独立的状态机。
5. 原生宿主是默认的执行环境。
6. 没有任何模型可见的路径能创建或放宽权限。
7. 当人类控制或触碰机器时绝不注入 agent 输入；Computer Fail Closed。
8. 模型观察与人工媒体保持为相互分离的平面。
9. agent 子进程绝不持有 app 的 TCC 授权。
10. CodexBridge 是位于适配层之后的实现来源，绝不是 CodeBridge 领域或 MCP 接口面的拥有者。
11. Fleet、cloud 与媒体基础设施保持可选；只有 TURN 中继可以位于默认个人路径上，且仅在直连媒体失败时使用。
