# CodeBridge V2 迁移方案

状态：**迁移基线 —— 由 2026-10-02 架构评审修订**

V2 之前的 V1 检查点：`d251a3c Build durable runtime and harness session foundation`。

V2 既不是全部重写，也不是原样延续 V1。它保留让 CodeBridge 与众不同的 Runtime 工作，从个人路径上退役 Manager 中继，并且只在 CodeBridge 自有的适配层之后复用上游桌面代码。

## 1. 原则

```text
Do not rebuild what a pinned, compatible upstream already solves well.
Do not throw away the durable Runtime that already works.
Do not let upstream internals become CodeBridge's domain model or MCP surface.
Do not carry V1 security shortcuts into V2.
```

## 2. V1 代码处置

事实来自 2026-10-02 代码评审（[总体架构](architecture.zh-CN.md) §3.1）。

### 2.1 保留并上提到 `codebridged`

| V1 代码 | V2 去向 | 需要的改动 |
| --- | --- | --- |
| `internal/agentops/run_manager.go` —— RunManager、状态机、并发、取消、超时、恢复到 `interrupted` | Runtime | 增加 session / project / provider-session / process-identity 字段 |
| `internal/agentops/run_store.go` | Runtime store | V2 schema；Application Support 位置；状态变更与事件在同一事务中写入；`schema_version` |
| `internal/agentops/run_events.go` —— journal、`SubscribeEvents`、`EventsAfter` | Runtime Journal | 泛化为 `stream` / `seq` + 全局 `pos`；原始事件的保留策略 |
| `internal/agentops/subagent.go` —— CLI harness 启动 | AgentProvider execution | 独立的进程组；process identity；autonomy-mode 映射 |
| `internal/agentops/harness_session.go`、`omp_session.go`、`codex_session.go`、`opencode_session.go` | AgentProvider 历史 + 实时归一化 | 把它们接起来；目前它们有测试但未被使用 |
| `internal/agentops/ops.go` 的文件/git/LSP 工具、`apply_patch` / `rollback_patch`、安全与敏感路径检查 | Project 之后的 Bridge tools | 通过 capability descriptors 重新注册 |
| `internal/manager/tool_output.go` 输出 schema | capability descriptors | 复用 schema 与校验模式 |

### 2.2 重写

| V1 代码 | 原因 |
| --- | --- |
| `internal/agentops/permissions.go` | 保留 deny > allow > ask 的匹配思路；重写为带 verbs、selectors、scopes、approval channels 和权限衰减（attenuation）的 Policy。V1 的授权（grant）是进程全局的，且可由模型签发 |
| `permission_grant` MCP tool | 移除：它可由模型调用、被声明为只读，并且能持久化 `always` 规则。V2 的审批只来自 `local_ui` 或显式启用的 `remote_human` |
| Client 本地配置 UI（`internal/ui`） | 已被 CodeBridge.app 取代 |

### 2.3 随 V1 退役（保持可构建，不再扩展）

| V1 代码 | 原因 | 后续可能的复用 |
| --- | --- | --- |
| `cmd/client/runtime_events.go`（`run_event`、`run_heads`、`run_replay`） | Journal 与 MCP server 共享同一进程；重连就是一次游标读取 | — |
| `internal/manager`（`Registry`、`grpc_agent.go`、`RunEventBroker`、admin API）、`internal/pb`、`internal/protocol` 信封 | Manager 中继不在 V2 路径上 | fleet：registry 背压、broker 重放逻辑 |
| `internal/authserver`、`internal/oauthresource`、`internal/authstore`、`internal/clientcred` | Tunnel 是远程边界；个人路径中不做设备注册 | fleet：注册、轮换、JWT 校验 |
| `internal/mcpcallstore` | 由 Journal/审计事件取代 | — |
| `internal/auditlog` | 仅元数据的原则保留；审计事件写入 Journal | 格式参考 |
| `Dockerfile.client`、`deploy/compose.client.yml`、`deploy/client-entrypoint.sh` | Docker 不是默认执行环境 | Phase 7 沙箱参考 |

V1 已支持原生客户端（`deploy/launchd`、`deploy/systemd`）；V2 放弃的是作为默认的 Docker，而不是原生执行。

## 3. 停止在主路径上继续扩展

冻结而不是删除：

- 对外的 Manager MCP 入口（ingress）以及 Manager 作为强制中继；
- Manager→Client gRPC 个人执行路径；
- Docker Client 作为 Mac 默认方式；
- Caddy/Nginx 公网 MCP 路由；
- 为中继而构建的自定义 OAuth 入口与设备注册/账号路由。

保留这些代码，直到 V2 证明替代路径可行（§10），然后在专门的清理里程碑中归档或删除。

## 4. CodexBridge 复用

### 4.1 现状：上游已定位，但尚未固定

目标上游现已从外部明确定位为 `Fanch-hui/codex-bridge`。其公开 `win` 分支已经描述了预期的原生桌面/服务架构、Secure MCP Tunnel、任务/会话、审批、Agent Discovery/Connectivity，以及 Apache-2.0 许可证。但本仓库仍没有 vendored 副本或固定的上游 commit，CodeBridge 也尚未完成上游原样构建。Phase 0 因此仍需先验证具体模块边界，再决定复用方式。

Secure MCP Tunnel 本身是 OpenAI 的 `tunnel-client`。CodeBridge 由 `codebridged` 监管它，并让它通过 Unix-domain socket 访问 daemon 的 Streamable HTTP MCP Endpoint，因此关键路径不依赖 CodexBridge，也不需要额外的 stdio shim。

### 4.2 能力矩阵（Phase 0 填写）

| 宣称的上游能力 | 需求方 | 首选复用方式 | 兜底方案 |
| --- | --- | --- | --- |
| 原生桌面外壳 | CodeBridge.app | 包依赖，或带署名的复制 | 自研 SwiftUI 菜单栏应用 |
| Secure MCP Tunnel 安装配置体验 | onboarding | 仅 UI；进程由 `codebridged` 监管 | 自研配置界面 |
| 打包 / 签名 / 公证 / 更新器 | release | 构建脚本与更新器框架 | 自研流水线 |
| 凭证 / Keychain | SecretStore | 模块 | 自研 Keychain 封装 |
| 审批 UI | ApprovalPresenter | 仅 UI 组件；决策仍留在 Policy | 自研 UI |
| 项目注册 | Project | 仅 UI；注册表属于 CodeBridge | 自研 UI |
| agent 发现 | AgentProvider `Describe` | 检测逻辑 | 自研检测 |

### 4.3 复用模式（按优先级排序）

1. **固定版本、不做修改的包依赖。**
2. **带署名的复制** —— 把一小块稳定的代码一次性移植到 third-party 目录，并记录来源 commit 与 NOTICE；不期待同步。
3. **带补丁队列的 vendor 子树** —— 仅用于需要修复的较大模块。每个补丁都小巧、有理由且可回流上游。
4. **直接运行或 fork 上游应用，并把 CodeBridge 插进去** —— 已否决。这会让 CodeBridge 变成上游的插件，并使 CodeBridge 的 MCP 面受上游控制。

已定位的上游使用 Apache-2.0，但 Phase 0 仍必须为每个复用模块记录 LICENSE / NOTICE 义务与准确的来源 commit，再决定具体复用模式。

### 4.4 适配层

```text
CodeBridge domain (Swift app code, Host IPC, Go daemon)
        |
        | CodeBridge-owned protocols:
        | ApprovalPresenting, SecretStoring, Updating, AgentDetecting, ProjectPicking
        v
Adapters (CodeBridge code)
        |
        v
Upstream-derived modules (unmodified where possible)
```

规则：

- 上游类型绝不进入 CodeBridge 领域代码或 Host IPC；
- CodeBridge 领域类型绝不进入上游代码；
- 上游代码绝不注册或路由 CodeBridge MCP tools，也绝不拥有 CodeBridge 的 session model。

### 4.5 向上游申请的扩展点

目标：零。至多：

1. 菜单/区块 provider 钩子；
2. 带外部决策回调的审批组件；
3. 可配置的 Keychain service name / access group；
4. 可配置的更新器 feed。

绝不申请：接入上游 MCP/tool 注册表或 session model 的钩子。

### 4.6 同步流程

- 在上游派生代码旁的 `UPSTREAM.md` 中记录仓库、commit、许可证、使用的模块和复用模式。
- 在专门的提交中同步，绝不与功能开发混在一起。
- 把每个上游 diff 都当作安全变更来评审：这些代码以 CodeBridge.app 的 TCC 授权运行。
- 每次同步后都构建并做冒烟测试。

### 4.7 身份

CodeBridge.app 有自己的 bundle id 和签名身份。TCC 授权绝不与上游应用共享。

## 5. 仓库目标形态

概念性的；实际构建可能需要不同的布局。依赖方向比目录名称更重要。

```text
codebridge/
├── cmd/
│   ├── codebridged/          # V2 daemon: Bridge kernel + domain services + Runtime
│   ├── manager/  client/     # V1, frozen
│   └── doctor/
├── internal/
│   ├── bridge/               # kernel: ingress, caller, capability registry, dispatch
│   ├── policy/  project/  secret/
│   ├── runtime/              # sessions, runs, journal, artifacts (lifted from agentops)
│   ├── agent/                # AgentProviders: omp, codex, opencode
│   ├── computer/             # ComputerSession broker; Host IPC client of the engine
│   ├── hostipc/              # Host IPC bindings
│   └── agentops/ manager/ …  # V1, frozen
├── schema/hostipc/v1/        # language-neutral Host IPC schema
├── desktop/macos/            # CodeBridge.app: UI, computer engine, media, adapters
│   └── ThirdParty/           # upstream-derived code + UPSTREAM.md
└── docs/v2/
```

过渡期间 V2 package 可以 import 上提的 V1 代码；V1 package 绝不 import V2 package。

## 6. 迁移阶段

与[开发路线](roadmap.zh-CN.md)对齐。

| 阶段 | 内容 |
| --- | --- |
| A —— 基线（Phase 0） | 固定或否决上游；技术验证（spike）（ingress、生命周期、TCC 归因、原生宿主、小组件媒体）；Host IPC v1 与 store schema v1 |
| B —— 基础（Phase 1A） | `codebridged`、Bridge 内核、Policy v2、V2 store 与 Journal、Tunnel 监管 |
| C —— Computer（Phase 1B–2） | computer engine、tools、E2E；随后是预览与远程接管 |
| D —— Runtime（Phase 3） | RunManager 上提、AgentProviders、ProviderSession、通知、V1 历史导入 |
| E —— 编排 / 云（Phase 4–5） | 仅在 Computer 与 Runtime 稳定之后 |

## 7. 数据迁移

决策：

- V2 在 `~/Library/Application Support/CodeBridge/` 中启用新的 store。
- V1 的 `runs.db`（位于用户缓存目录）绝不被修改。Phase 3 提供一次性的只读导入，按 workspace 导入为一个 `imported` Session。
- 不对 V1 数据行做隐式重新解释；导入是显式且带版本号的。

## 8. 配置迁移

```text
CODEBRIDGE_MANAGER_HOST       -> removed from personal configuration
workspaces / writable flags   -> Projects
bash / agent permission rules -> Policy rules (verbs + selectors); "always" rules re-confirmed locally
subagent_profiles             -> AgentProvider profiles / ModelPolicy
OAuth / enrollment settings   -> not migrated (tunnel is the remote boundary)
```

密钥绝不在存储系统之间自动复制。

## 9. 测试策略

三个层次：

1. **契约测试** —— Bridge 内核、Policy、Runtime Journal、Host IPC schema；不涉及真实 OS 或 Provider。
2. **Provider 测试** —— fixtures 与 fake，外加有针对性的真实冒烟（harness CLI；真实 Mac 上的 ScreenCaptureKit / CGEvent）。
3. **端到端** —— 在原生宿主上通过 Tunnel 使用真实 ChatGPT。

Computer 安全测试是强制性的，并尽可能做到确定性：controller 变更后的过期帧（frame）、几何变化、本地输入抢占、受保护界面、范围外应用、租约（lease）过期、应用退出、daemon 重启、IPC 版本不匹配。

最高优先级 E2E：

```text
ChatGPT -> Secure MCP Tunnel -> codebridged -> CodeBridge.app
  -> computer_observe -> computer_action -> computer_observe
```

其次：

```text
ChatGPT -> agent_start -> durable local run -> disconnect / app quit / daemon restart
  -> continue or interrupted + resumable -> reconnect -> replay / final result
```

## 10. 提交策略

可评审的独立提交：

- 上游导入或同步（绝不与功能混合）；
- schema 与契约；
- daemon 基础；
- computer engine；
- computer tools 与 E2E；
- runtime 上提；
- 编排；
- 云。

## 11. 回滚规则

在 V2 达到 Phase 1 退出标准之前：

- V1 从冻结的检查点（checkpoint）保持可构建；
- 不对 Manager 或 Client 代码做破坏性删除；
- V2 的改动可按分支或提交回滚；
- 上游同步彼此隔离。

在 V2 证明替代路径可行之后，V1 的部署组件将在专门的清理里程碑中归档或移除。
