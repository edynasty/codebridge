# CodeBridge V2 迁移方案

状态：**迁移基线**

建立 V2 文档前的 V1 Checkpoint：

`d251a3c Build durable runtime and harness session foundation`

目标既不是全部重写，也不是继续 V1 原架构。V2 复用 CodexBridge 已成熟的 Bridge/Desktop 基础能力，同时保留 CodeBridge 已完成且真正有差异化价值的 Runtime。

## 1. 迁移原则

```text
上游已经成熟解决的，不重复造。
已经做好的 Durable Runtime，不丢。
上游内部类型，不能成为 CodeBridge 领域模型。
```

CodexBridge 是上游实现来源；CodeBridge 保留独立 Architecture 与 Provider Contract。

## 2. 当前 CodeBridge 必须保留/迁移

### Runtime

- RunStore；
- RunManager；
- Durable Ordered RunEvent Journal；
- Subscription；
- Reconnect / Replay 思路；
- Async Run Ownership；
- Terminal Result Persistence。

### Harness / Session Adapter

- HarnessAdapter；
- OMP Live / Session Parser；
- Codex Live / Session Parser；
- OpenCode Session 工作；
- Normalized Display Projection；
- Raw Local Event Retention。

### Computer Design

保留正确决策：

- 模型静态截图与人类实时视频分离；
- Logical Coordinate；
- Explicit ComputerSession；
- Exclusive Controller；
- Human Takeover；
- WebRTC / TURN 分离；
- Permission Namespace。

但旧设计中对 Manager / gRPC 的依赖不再是架构硬约束。

## 3. 主路径停止继续扩展

先 Freeze，不立即删除：

- Public Manager MCP Ingress；
- Manager Mandatory Relay；
- Manager → Client gRPC 个人执行链；
- Docker Client 作为 Mac 默认；
- Caddy/Nginx Public MCP；
- 自建 OAuth Ingress；
- 主要为了旧 Relay 服务的 Device Enrollment / Account Routing。

V2 替代链路验证前保留源码；验证后再专门 Archive / Cleanup。

## 4. 从 CodexBridge 上游复用

目标：

- macOS Desktop App Shell；
- 后续 Windows Desktop Shell；
- Secure MCP Tunnel；
- Local Service / App Lifecycle；
- Packaging / Signing / Updater；
- Keychain / Credential Manager；
- Project Registration；
- Approval UI；
- Agent Discovery；
- 可复用的 Agent Connectivity；
- 兼容的 Workspace Security。

正式 Fork / Import 前：

- 审计模块边界；
- 审计 License / NOTICE；
- 找最小扩展点；
- 原样构建上游；
- 跑上游 Test / Smoke；
- 记录 Upstream Tag / Commit。

## 5. Thin Upstream Rule

禁止：

```text
fork
→ 到处改
→ 后续同步地狱
```

目标：

```text
upstream CodexBridge
       |
       +-- 少量 CodeBridge Hook
               |
               +-- Runtime Adapter
               +-- Computer Provider
               +-- Event Sink
               +-- UI Extension
```

上游升级冲突应集中在很少的 Integration 文件。

能放在上游 Core 之外实现的能力，必须放外面。

## 6. 目标仓库结构

概念上：

```text
codebridge/
├── upstream/
│   └── codexbridge/
├── core/
│   ├── bridge/
│   ├── session/
│   ├── event/
│   ├── permission/
│   └── artifact/
├── providers/
│   ├── transport/
│   ├── agent/
│   ├── computer/
│   ├── media/
│   ├── cloud/
│   ├── snapshot/
│   └── controlplane/
├── runtime/
├── desktop/
└── docs/v2/
```

实际物理目录可以受上游构建系统影响；最重要的是依赖方向。

## 7. 迁移阶段

### Stage A — Baseline

1. Freeze 当前 V1 Checkpoint；
2. 建立 V2 Doc / Contract；
3. 审计 CodexBridge；
4. 原样构建上游；
5. Native Mac 验证 Tunnel / Project / Shell / Docker。

### Stage B — Integration Seam

1. RuntimeProtocol v1；
2. Provider Registration；
3. EventSink；
4. ID / Schema Version；
5. 少量上游 Hook；
6. Native Service ↔ Go Runtime Health Handshake。

### Stage C — Computer-first

1. MacComputerProvider；
2. Observe；
3. Input；
4. ComputerSession；
5. Permission；
6. ChatGPT Real E2E；
7. WebRTC / Takeover。

### Stage D — Runtime Migration

1. 迁移 RunStore / Journal；
2. 旧 Run 映射 V2 Session / Run / Turn / Event；
3. 迁移 Agent Provider / Adapter；
4. Durable Recovery；
5. Runtime UI / Notification。

### Stage E — Orchestration / Cloud

Computer + Runtime 稳定以后再做。

## 8. 数据迁移

禁止静默解释旧数据库。

必须显式 Schema Version。

可选方案：

- 读取旧 `runs.db`，做一次迁移；
- 旧 DB 只读，V2 新建 DB；
- 导出/导入选择性的 Run History。

等 V2 Storage Schema 冻结后再决定。

## 9. 配置迁移

转换旧概念，不保留旧部署假设。

例如：

```text
CODEBRIDGE_MANAGER_HOST
    -> 个人默认配置移除

workspaces
    -> Project / Workspace Authorization

permissions
    -> Namespace Permission

subagent_profiles
    -> AgentProvider Profile / ModelPolicy
```

Secret 禁止在不同 Storage 之间自动明文复制。

## 10. 测试策略

三层：

1. **Core Contract Test**：不用真实 OS / Provider。
2. **Provider Test**：Fixture/Fake + 定向本地 Smoke。
3. **Real E2E**：关键链路使用 ChatGPT/Tunnel/Native Host。

最高优先真实 E2E：

```text
ChatGPT
 -> Secure MCP Tunnel
 -> Native CodeBridge
 -> computer_observe
 -> computer_action
 -> computer_observe
```

第二：

```text
ChatGPT
 -> agent_start
 -> local durable run
 -> disconnect
 -> continue locally
 -> reconnect
 -> replay/final result
```

## 11. Commit Strategy

迁移必须保持可 Review：

- Baseline / Upstream Import；
- Extension Contract；
- Native Bridge Integration；
- Computer MVP；
- Runtime Migration；
- Orchestration；
- Cloud。

一次 Commit 禁止同时混合 Upstream Sync 和大规模 CodeBridge Feature Change。

## 12. Rollback Rule

V2 Native Computer E2E 完成前：

- V1 Frozen Checkpoint 必须仍然可构建；
- 不破坏性删除 Manager/Client；
- V2 必须可以通过 Branch/Commit 回退；
- Upstream Sync 单独隔离。

V2 替代链路验证后，再用独立 Cleanup Milestone 删除/归档旧部署组件。
