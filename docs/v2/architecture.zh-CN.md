# CodeBridge V2 总体架构

状态：**架构基线 / 长期目标**

## 1. 产品定义

CodeBridge 是 AI Client 与真实开发环境之间的本地连接、执行与控制层。

V2 有三个地位相同的一级核心域：

- **Bridge**：负责传输、项目/工作区、能力路由、权限、审批、凭证、工具与 Skills。
- **Runtime**：负责 Session、Run、Turn、持久事件、Artifact、恢复、调度与长任务。
- **Computer**：负责桌面观察、输入控制、实时预览、控制权和后续自动化能力。

架构原则：

> **Bridge-first architecture，Computer-first roadmap。**

Bridge 是架构中枢；Computer Use 是当前最高的新功能开发优先级。

## 2. 设计目标

V2 必须做到：

- 默认直接运行在真实宿主机，而不是 Docker 中的近似环境；
- AI 对外协议不绑定 OMP、Codex、OpenCode、macOS 或某一个 Tunnel；
- ChatGPT、Tunnel、UI 断开时，本地长任务仍可继续；
- Computer Use 与 Agent 共用同一套权限、Session、Event 和 Runtime；
- 为未来 Local ↔ Cloud 接力预留稳定接口，但核心不绑定 Codex Cloud；
- 尽量复用 CodexBridge 已成熟的桌面、Tunnel、权限、打包能力；
- 保留 Windows、Linux、Fleet、其他 AI Client 和其他 Transport 的扩展空间。

## 3. 总体结构

```text
+----------------------------------------------------------+
|                        AI Clients                        |
| ChatGPT / Codex / future AI client / local MCP client    |
+-----------------------------+----------------------------+
                              |
                       Transport Layer
                              |
                 +------------+-------------+
                 |            |             |
                 v            v             v
          Secure MCP      Local MCP      Future
             Tunnel       Transport      Transport
                 \            |            /
                  +-----------+-----------+
                              v
+----------------------------------------------------------+
|                    CodeBridge Core                       |
|                                                          |
| Capability Router                                        |
| Project / Workspace                                      |
| Session                                                  |
| Permission / Approval                                    |
| Identity / Credential                                    |
| Skills / Tools                                           |
| Event Bus                                                |
| Extension Registry                                       |
+-----------+------------------+-------------------+--------+
            |                  |                   |
            v                  v                   v
+------------------+  +------------------+  +------------------+
| Agent Runtime    |  | Computer Runtime |  | Cloud Runtime    |
| OMP/Codex/...    |  | Observe/Input    |  | Provider/Handoff |
+---------+--------+  +---------+--------+  +---------+--------+
          \                   |                     /
           +------------------+--------------------+
                              v
                     Persistent Runtime
                Session / Run / Turn / Event
                 Artifact / Journal / Recovery
                              |
                              v
+----------------------------------------------------------+
|                    Native Host Layer                     |
| Files / Shell / Docker / Git / IDE / Browser / VPN       |
| kubectl / SSH / local DB / desktop / local services      |
+----------------------------------------------------------+
```

## 4. 进程边界

V2 本地主要由两个进程组成。

### 4.1 Native Desktop Service

macOS 优先使用 Swift，实现上尽量复用 CodexBridge 已成熟组件。

职责：

- Secure MCP Tunnel；
- Local MCP 入口；
- 桌面 UI；
- macOS 权限；
- Keychain / Credential；
- 项目注册和审批 UI；
- macOS ComputerProvider；
- WebRTC / 原生媒体；
- Updater / Packaging；
- 管理 Go Runtime 生命周期。

### 4.2 CodeBridge Runtime

沿用并演进当前 Go Runtime。

职责：

- Session / Run 持久状态；
- 有序 Event Journal；
- AgentProvider；
- Orchestration；
- Artifact 元数据；
- Recovery；
- Workspace Checkpoint 元数据；
- 后续 Cloud Handoff；
- 本地 SQLite Runtime 状态。

Swift 与 Go 之间必须通过**有版本的本地 IPC 协议**通信，禁止直接把内部结构当跨进程协议。

## 5. 统一领域模型

Agent、Computer、Cloud 共用唯一生命周期：

```text
Project
└── Session
    ├── Run
    │   ├── Turn
    │   ├── Event
    │   └── Artifact
    ├── AgentContext
    ├── ComputerContext
    └── CloudContext
```

定义：

- **Project**：已授权的项目/Workspace。
- **Session**：可跨多个 Run 持续存在的工作上下文。
- **Run**：一个明确目标或任务。
- **Turn**：一次有边界的 Agent / Computer / Cloud 执行。
- **Event**：一次有序状态变化。
- **Artifact**：文件、Patch、Commit、报告、测试结果、截图引用、Cloud 结果等。

禁止 Computer、Cloud、某个 Harness 再创建独立生命周期体系。

## 6. Event 主干

全系统重要状态变化统一产生 `RuntimeEvent`：

```text
RuntimeEvent
  id
  project_id
  session_id
  run_id
  seq
  timestamp
  source
  kind
  correlation_id
  payload
```

source：

- bridge
- runtime
- agent
- computer
- cloud
- media
- system

典型 kind：

- `run.started`
- `agent.message`
- `agent.tool.started`
- `computer.observed`
- `computer.action`
- `computer.controller.changed`
- `cloud.snapshot.ready`
- `cloud.handoff.started`
- `artifact.created`
- `run.completed`

本地 Journal 是恢复的权威数据源；实时推送只是缓存/通知路径。

## 7. Provider 模型

具体实现全部通过 Provider 接入。

必须预留：

- `TransportProvider`
- `AgentProvider`
- `ComputerProvider`
- `MediaProvider`
- `CloudProvider`
- `SnapshotProvider`
- `SkillProvider`
- `ToolProvider`
- `ControlPlaneProvider`（未来 Fleet）

Bridge Core 不允许直接依赖 OMP、Codex、OpenCode、macOS、WebRTC、Codex Cloud 或 CodexBridge 内部类型。

## 8. Bridge Core

Bridge Core 是真正的架构中枢，负责：

- Capability Discovery / Routing；
- Project / Workspace 授权；
- Permission；
- Approval；
- Session Resolution；
- Tool / Skill 注册；
- Provider 注册；
- 与 Transport 无关的请求处理；
- Event 发布。

标准调用链：

```text
AI Request
  -> TransportProvider
  -> Bridge Core
  -> Permission / Session
  -> Capability Router
  -> Provider
  -> Runtime Event / Artifact
  -> Response
```

Computer 和 Agent 都不能绕过 Bridge Core。

## 9. Native Host 默认策略

个人开发者默认执行环境就是实际 OS。

因此 Agent 可以自然访问：

- Docker Desktop；
- brew / package manager；
- launchctl / system service；
- IDE；
- SSH；
- kubectl；
- VPN / 内网；
- 本地数据库；
- 浏览器和真实桌面。

Container / Sandbox 是可选 Provider 或 Policy，不再作为 Mac 开发机默认部署方式。

## 10. Computer 模型

Computer 是一级能力：

```text
ComputerSession
  id
  project_id
  session_id
  target
  state
  controller
  controller_epoch
  permissions
  lease
  capabilities
  last_frame_id
```

Target 从一开始就可扩展：

- display
- window
- application
- browser
- virtual desktop

V1 只实现安全且必要的类型。

Controller：

- agent
- human
- none

每个 input action 必须携带 `controller_epoch`。控制权切换时 epoch +1，旧 epoch 排队中的操作自动拒绝。这是 Human Takeover 的核心并发屏障。

## 11. Computer 数据面

模型观察与人类直播永久分离：

```text
Computer Runtime
     |
     +-- Observation -> MCP image -> Model
     |
     +-- Media -> WebRTC -> Human Widget
```

规则：

- 模型只拿显式静态截图；
- Live Video 永远不进入模型上下文；
- Computer Action 不依赖 WebRTC；
- Preview 挂掉不能破坏 ComputerSession；
- 默认不持久化截图/视频。

## 12. Computer Action

两级扩展模型。

### Primitive

- move
- click
- double_click
- drag
- scroll
- type
- keypress
- wait

### Automation（预留）

- script
- browser_script
- workflow

协议必须允许未来新增 Action Kind，而不修改 Session 模型。

## 13. Runtime 可靠性

本地 Run 属于 Runtime，不属于某一次 ChatGPT 请求。

以下事件不能自动结束 Run：

- ChatGPT Turn 结束；
- 浏览器关闭；
- Tunnel 重连；
- Desktop UI 重启。

恢复依赖 Event Journal 和 Provider 原生 Session 状态。

## 14. Cloud Handoff

Cloud 只是 Provider，不是 Core 依赖：

```text
Local Run
  -> Incremental Checkpoint
  -> WorkspaceSnapshot
  -> CloudProvider
  -> Cloud Run
  -> Artifact / Diff
  -> Resume Local
```

Core Contract 必须能支持：

- Codex Cloud；
- OpenAI Hosted；
- Self-hosted；
- 未来其他 Provider。

WorkspaceSnapshot 概念上包含：

- base revision；
- working-tree diff；
- untracked manifest；
- environment manifest；
- runtime continuation summary；
- artifact references。

上传实现可以以后做，但接口现在固定。

## 15. Agent Runtime 与 Orchestration

OMP、Codex、OpenCode 都只是 `AgentProvider` 实现。

未来 Orchestrator 与 Provider 解耦：

```text
Orchestrator
├── Role
├── TaskGraph
├── Delegation
├── ReviewLoop
├── ModelPolicy
└── BudgetPolicy
```

Role 最终可映射到任意 Provider / Model，支持跨 Provider 编排。

## 16. 安全模型

权限使用 Namespace：

- `filesystem.read`
- `filesystem.write`
- `shell.execute`
- `agent.start`
- `agent.cancel`
- `computer.observe`
- `computer.input`
- `computer.preview`
- `cloud.upload`
- `cloud.execute`
- `network.connect`
- `secret.read`

Effect：

- allow
- deny
- ask

Scope：

- once
- session
- project
- always

deny 永远优先。

远端调用者不能扩大本机权限。

## 17. 存储边界

本地可以持久化：

- Project / Session / Run 元数据；
- Event Journal；
- Provider Session Locator；
- Artifact 元数据；
- Terminal Result；
- Cloud Checkpoint 元数据；
- Local UI Index。

默认禁止持久化：

- 屏幕录像；
- 截图；
- Secret；
- 纯遥测目的的完整源码副本；
- 模型隐藏推理。

## 18. CodexBridge 上游关系

CodexBridge 是**实现复用来源**，不是 CodeBridge 领域架构依赖。

优先复用：

- Native Desktop Shell；
- Secure MCP Tunnel；
- Packaging / Updater；
- Credential；
- Approval UI；
- Project Registration；
- Agent Discovery。

CodeBridge 差异代码必须尽量集中在少量 Integration Hook：

- RuntimeProvider；
- ComputerProvider；
- EventSink；
- UI Extension。

禁止大面积侵入上游模块。

## 19. Future Fleet

Fleet 不进入个人版 V2 主路径。

预留 `ControlPlaneProvider`：

- `LocalControlPlane`：默认；
- `FleetControlPlane`：未来。

以后可增加设备资产、企业策略、集中审计、版本和健康管理，但不能重新成为个人 MCP/Computer 流量必经节点。

## 20. 架构硬规则

1. Bridge Core 不依赖具体 Agent、OS、Transport、Media、Cloud。
2. Computer、Agent、Cloud 全部必须经过 Bridge Core。
3. Session / Run / Turn / Event / Artifact 是唯一生命周期模型。
4. Native Host 是个人开发者默认执行环境。
5. 本地 Durable State 是恢复权威数据源。
6. 模型截图与人类视频永久分离。
7. CodexBridge 是可复用上游，不是 CodeBridge 架构所有者。
8. 新功能原则上通过 Provider / Extension 增加，不修改 Core 模型。
