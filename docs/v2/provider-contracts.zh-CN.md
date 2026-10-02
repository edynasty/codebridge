# CodeBridge V2 Provider 接口

状态：**契约基线 —— 经 2026-10-02 架构评审修订**

本文件规定功能从何处接入 V2、当前冻结哪些契约，以及哪些契约有意保持未定义。语言级的确切签名将在 Phase 0/1 中编写；职责划分、依赖方向以及下文规则具有约束力。

## 1. 契约分层

| 层级 | 含义 | 契约 |
| --- | --- | --- |
| **冻结（Frozen）v1** | 仅在 V2 内做加法式变更 | AgentProvider；ComputerProvider（Host IPC `computer` 服务）；Host IPC envelope 与握手；capability descriptor；event emission；artifact；error model |
| **内部端口（Internal port）** | `codebridged` 内部的 Go 接口；可自由变更 | ingress adapters、SecretStore、ApprovalPresenter、Notifier、TunnelSupervisor |
| **延后（Deferred）** | 已命名、有意保持未定义；只有约束性条件 | CloudProvider、workspace checkpoint、standalone MediaProvider、SkillProvider、ControlPlane / Fleet、sandbox execution |

为什么冻结的契约族比之前的九个更少：今天或 Phase 1 内只有 AgentProvider 和 ComputerProvider 有实现。冻结一个没有实现的接口，等于冻结猜测——例如，一个“上传快照”式的 CloudProvider 与 Codex Cloud 基于 GitHub 的环境并不匹配，而 TransportProvider 与 `tunnel-client` 和 MCP transports 现有能力重复。

## 2. 依赖规则

```text
Bridge kernel ──> domain services ──> provider ports <── provider implementations
                                      (Go interface or Host IPC schema)
```

1. Provider 只导入自己的 port、event emission 契约和 error model。
2. **任何 Provider 都不调用另一个 Provider。** 组合属于领域服务（例如 Runtime 协调 checkpoint 与 cloud），或属于 Bridge 请求（需要 Computer 的 Agent 以 `agent_internal` 身份调用 CodeBridge 工具）。
3. 跨进程 Provider 只能通过带版本的 Host IPC 访问。IPC schema 是语言无关的，存放在仓库中；绝不从 Swift 或 Go struct 生成。
4. Provider 绝不决定 Policy。它们上报 capabilities 与 autonomy，由 Policy 决定。
5. Provider 绝不持久化 CodeBridge 生命周期真相。它们发出 Event；由 Runtime 提交。
6. Bridge 内核和领域服务绝不导入具体 Provider、平台 SDK 或上游（upstream）模块。

## 3. Provider 通用要素

每个 Provider 都暴露：

- **Describe** —— provider id、实现版本、契约版本、显式 capability flags、limits；
- **Health** —— `ok | degraded | unavailable`，并附带原因；
- **Errors** —— §11 中的分类；
- **Cancellation** —— 被及时响应；
- **Idempotency key** —— 在调用方可能重试的变更类操作上被接受。

## 4. Capability descriptor（冻结 v1）

Bridge 暴露的每个工具都声明：

- **name** —— 稳定；破坏性变更要换用新名称；
- **input and output schema** —— output schema 是强制的（V1 已在 `internal/manager/tool_output.go` 中声明）；
- **permission verb 与 selector extractor** —— 资源如何从参数中推导出来；
- **risk class** —— `read | write | execute | control | egress`；
- **allowed caller classes** —— 例如仅 UI 可用的工具；
- **scope requirements** —— project、session 或 computer session；
- **result visibility** —— 模型可见内容与仅 widget 可见的 `_meta` 之别；
- **idempotency** —— 可安全重试，或需要 key。

工具由领域服务注册到 Bridge 内核的 capability registry。不存在独立的 ToolProvider 契约族。

## 5. AgentProvider（冻结 v1）

用途：把一个 Agent harness 适配为 Runtime Run。

```text
Describe        -> capabilities {resume, history, live_events, cancel, reattach, approval_routing},
                   autonomy modes, models
Start(run_id, project root, task, model, role?, autonomy, provider_session_ref?)
                -> Execution
Execution
  Events        stream of ProviderEvent {normalized kind?, turn_ref?, raw (local only),
                                         provider_session_ref when bound}
  Wait          -> Outcome {status, final output, error category}
  Cancel
  Process       -> process identity {pid, start time, process group}
Reattach(provider_session_ref, process identity) -> Execution      [optional: reattach]
ListSessions / ReadSession(ref, cursor)                             [optional: history]
```

规则：

- **尽早绑定。** 一旦实时输出暴露出 provider 原生 session id，Provider 就立即绑定。时间/任务启发式只是回退方案，且必须标记为回退。
- **Resume** 即带 `provider_session_ref` 的 `Start`。Runtime 保证每个 ProviderSession 至多只有一个活跃 Run。
- **归一化与原始数据。** Core 只依赖归一化 kind（`message`、`tool_call`、`tool_result`、`file_change`、`command`、`error`、`runtime`）。原始记录保留在本地。
- **真实的 autonomy。** Provider 声明每种 autonomy 模式在该 harness 上的确切含义（例如 OMP `--auto-approve` = 完全宿主访问；Codex `--full-auto` = Codex 自带的沙箱 workspace-write 模式），并且绝不比被授予的模式更宽松地运行。
- **进程卫生。** Harness 运行在自己的进程组中；上报进程标识，以便恢复流程能发现孤儿进程。
- **集成机制属于私有实现。** CLI `exec`、Codex `app-server` JSON-RPC 或某个 SDK，都是 Provider 内部选择，绝不改变本契约。

V1 映射：`internal/agentops/subagent.go` 中的启动代码与各 `HarnessSessionAdapter` 实现（`List` / `Discover` / `Read` / `NormalizeLive`）合并为每个 harness 一个 AgentProvider。

实现：OMP、Codex 与 OpenCode（Phase 3）；其他后续。

## 6. ComputerProvider（冻结 v1）

用途：在权威的 input arbiter 之后暴露 computer target。在 macOS 上，这就是由 CodeBridge.app 实现的 Host IPC `computer` 服务。

```text
describe          -> platform, capabilities {observe, input, preview, ax_tree, targets[]},
                     permission states (screen recording, accessibility, input monitoring)
targets           -> displays (Phase 1); windows and applications as filters / scope
open_session(cmp, target, grants {verbs, app selectors}, lease)        -> session state
observe(cmp)      -> frame {frame_id, image, geometry, arbiter_instance,
                            controller_epoch, geometry_generation}
act(cmp, frame_id, actions[], observe_after)  -> per-action results, optional frame
control(cmp, op, actor)                       -> controller state
    op ∈ acquire_agent | pause | takeover | resume_agent | release
close_session(cmp)
notifications     -> controller.changed, preempted, permission.changed,
                     display.changed, session.state
```

Preview（Phase 2）是同一服务的可选 capability：`preview.start`、`preview.signal`、`preview.stop`，远端人工输入通过 preview data channel 传输。

每个实现都必须：

- 在同一进程内运行 arbiter 与 injector，且只有一个串行化的 injector；
- 按 [Architecture](architecture.zh-CN.md) §11.5 的规定给帧（frame）打标并校验；
- 实现本地输入抢占、受保护界面以及被保持输入的释放；
- 只接受来自人类 actor 的 `resume_agent`（退出人工保持（human hold））——本地 UI 或远端控制 grant——绝不来自模型路径；
- 遇到未知状态时 Fail Closed（失败即关闭）；重启后是新的 `arbiter_instance`，且 `controller = none`；
- 绝不持久化帧（frame）。

实现：MacComputerProvider（Swift，位于 CodeBridge.app）在 Phase 1；Windows 与 Linux 后续，可用任何语言，只要位于同一 IPC 契约之后。

## 7. Host IPC v1（冻结的 envelope）

- **Transport:** 位于 `0700` 每用户目录中的 Unix domain socket（Windows 上为 named pipe）。`codebridged` 监听；CodeBridge.app 连接。
- **Framing:** 长度前缀的 JSON-RPC 2.0，双向，图像走二进制 attachment frame。控制消息保持 JSON。
- **Handshake:** `host.hello {protocol: {major, minor}, app_version, role, capabilities[]}` → `{accepted, protocol, daemon_version}`。
- **Services:** `host`（hello、health、prepare_restart）、`computer`（§6）、`approval`（present、decision）、`notify`（post）、`runtime`（按 cursor 读取读模型，以及用户命令，以 `local_ui` 调用方身份发出）。
- **Compatibility:** minor 版本只做加法式变更；N 与 N-1 两个 minor 版本可互操作；未知字段被忽略；未知方法返回 `unsupported`；major 版本不匹配会禁用 app 的各项服务。
- **Authentication:** 对端 UID 校验；对声称 app role 的对端做代码签名校验。
- **Schema location:** 语言无关的 schema 存放在仓库中（由 Phase 0 决定格式）；Go 与 Swift 绑定由它生成，或按它手写。

## 8. 内部端口

这些是 `codebridged` 内部的 Go 接口，不是公开扩展点：

- **Ingress adapters** —— 远程 MCP 由 `tunnel-client` 通过 daemon 的 Unix-domain socket 上的 Streamable HTTP 接入；可选本地 adapter；为 `local_ui` 提供 Host IPC。
- **SecretStore** —— macOS 上的 Keychain。
- **ApprovalPresenter** —— 通过 Host IPC 连接 CodeBridge.app；用于 `remote_human` 审批的 widget。
- **Notifier** —— 通过 app 发送本地通知。
- **TunnelSupervisor** —— 启动、监控并重启 `tunnel-client`。

## 9. Event emission（冻结 v1）

Provider 发出 Event；由 Runtime 分配 `stream`、`seq` 和 `pos` 并提交。跨进程 Provider 通过 IPC notification 发出 Event，由 daemon 记入 Journal（日志）。任何 Provider 都不得维护与权威相竞争的 history。envelope 定义见 [Architecture](architecture.zh-CN.md) §8。

## 10. Artifact 契约（冻结 v1）

种类：`file`、`patch`、`commit`、`branch_ref`、`report`、`test_result`、`log_bundle`、`cloud_result`、`saved_screenshot`（仅在用户显式操作之后）。

字段：id、producer（run 或 computer session）、kind、reference、digest、size、sensitivity、retention class。大载荷存放于本地 artifact 目录，或置于外部引用之后。

## 11. 失败语义（冻结 v1）

| 分类 | 含义 |
| --- | --- |
| `unavailable` | Provider 或宿主能力不可达 |
| `permission_denied` | Policy 或 OS 权限拒绝该操作；缺少宿主/TCC 访问时使用稳定 reason `host_permission_required` |
| `approval_required` | Policy 判定为 `ask`；在模型可见输出中携带 approval id，绝不携带 approval token |
| `unsupported` | 未提供该 capability 或 action kind |
| `invalid_state` | 该操作此刻无效；携带原因：`controller_changed`、`stale_frame`、`geometry_changed`、`lease_expired`、`protected_surface`、`out_of_scope`、…… |
| `conflict` | 结果无法干净地应用（例如云端 diff） |
| `transient` | 重试可能成功 |
| `terminal` | 重试不会成功 |
| `cancelled` | 被调用方、用户或 Policy 取消 |

重试与恢复由 Core 决定。Provider 不得运行隐藏的无限重试循环。

## 12. Capability 发现

每个 Provider 都返回显式 capability flags。Core 绝不从 Provider 类型或操作系统推断 capability，因为权限可能在运行时禁用截屏或输入。

```text
Computer:  observe=true  input=true  preview=false  ax_tree=false  input_monitor=true
Agent:     resume=true   history=true  reattach=false  approval_routing=false
```

不受支持的高影响 capability 会显式失败；绝不静默降级。

## 13. 版本管理

- Provider 契约带有契约版本。
- Host IPC 协商 `major.minor` 与 capability flags（§7）。
- store 携带 `schema_version`，迁移只向前。
- 可选字段向后兼容；必填字段的变更需要新的 major 版本。

## 14. 延后（Deferred）契约及其约束性条件

### 14.1 CloudProvider

Phase 5 之前不予定义。现在就有约束力的内容：

- 接续是一个带 `continues_run_id` 的新 Run，绝不是被迁移的 run；
- 不假设存在 SSH、VM 或上传的环境；
- 结果以 artifact 形式返回，并通过显式操作在本地应用；
- 上传需单独授权（grant）；
- Codex Cloud 基于 GitHub 仓库：交接意味着推送一个检查点（checkpoint）分支、提交任务并拉取 diff。

### 14.2 Workspace checkpoint（原 SnapshotProvider）

一个基于 git 的 Runtime/Project 内部服务（base commit + 私有 ref 上的 WIP commit、白名单化的未跟踪文件、排除敏感路径）。只有当真实的 cloud target 需要第二种检查点（checkpoint）格式时，它才成为 provider。

### 14.3 MediaProvider

Phase 2 的 preview 是 ComputerProvider 的一项 capability，因为视频来自同一条采集管线，且人工输入必须到达同一个 arbiter。只有当存在第二种媒体来源时，才定义独立的 MediaProvider。

### 14.4 SkillProvider

已从基线中移除。Skill 属于 Agent harness。

### 14.5 ControlPlane / Fleet

已移除。未来的 fleet 是可选的出站连接器；托管 Policy 只能做限制。

### 14.6 Sandbox execution

Phase 7。以 AgentProvider 以及 shell 工具（container 或 VM）的 autonomy/environment capability 形式表达，绝不作为默认。

### 14.7 TransportProvider

已移除。MCP transports 和 `tunnel-client` 已经是 transport；CodeBridge 只需要入口（ingress）adapters（§8）。非 MCP 的 transport 会是一个新的 ingress adapter，而不是一个 Provider 契约族。

## 15. 架构约束

1. Provider 不能为外部发起的操作绕过 Policy。
2. Provider 通过 Runtime 发布 Event，绝不持有相竞争的生命周期真相。
3. 入口（ingress）adapters 绝不直接调用 Provider。
4. ComputerProvider 绝不对 ChatGPT 做认证，也绝不决定 Policy；它执行 controller、帧（frame）与 scope 规则。
5. cloud provider 绝不成为一个 run 的本地真相来源；它只返回 artifact。
6. Provider 原始数据可以本地保留；Core 只依赖归一化字段。
7. 源自上游（upstream）的代码通过 adapters 实现 CodeBridge 的 port；绝不定义它们。
