# CodeBridge V2 Provider 接口

状态：**接口基线**

本文件规定未来功能应该从哪里接入 V2。这里首先冻结职责和依赖方向；具体 Go/Swift 方法签名可以在实现阶段调整。

## 1. 依赖规则

```text
Bridge Core
   |
   +--> Provider Interface
           ^
           |
     Concrete Provider
```

具体 Provider 可以依赖 OS SDK、第三方项目和外部 API；Bridge Core 不可以。

## 2. TransportProvider

用途：把 AI Client 或本地调用者连接到 Bridge Core。

职责：

- Start / Stop；
- 暴露 Tool / Capability 元数据；
- 接收已认证请求；
- 支持的 Response / Event Streaming；
- Transport Health。

计划实现：

- `SecureMCPTunnelTransport`：个人远程默认；
- `LocalMCPTransport`：本地开发与测试；
- 后续 HTTP / 其他 Transport。

Transport 不处理业务路由和具体 Provider 执行。

## 3. AgentProvider

用途：把一个本地或远程 Agent Harness 接入统一 Runtime。

概念能力：

```text
Capabilities
Start
Resume
Cancel
Status
Sessions
Events
Artifacts
Health
```

计划：

- OMPProvider
- CodexProvider
- OpenCodeProvider
- 后续 OpenAIAgentsProvider
- CustomProvider

Provider 保留 Harness 原始历史，Core 只依赖稳定的归一化概念。

## 4. ComputerProvider

用途：暴露可控制的真实/虚拟 Computer Target。

概念能力：

```text
Capabilities
Targets
StartSession
Observe
Execute
Pause
Takeover
ResumeAgent
Stop
Health
```

Target 可扩展：

- display
- window
- application
- browser
- virtual

计划：

- 第一阶段 MacComputerProvider；
- 后续 WindowsComputerProvider；
- 后续 LinuxComputerProvider。

ComputerProvider 只负责 OS 交互，不负责 MCP Transport。

## 5. MediaProvider

用途：独立传输给人看的实时媒体，不与模型截图耦合。

概念能力：

```text
StartPreview
CreateViewerGrant
Signal
UpdateQuality
StopPreview
Health
```

计划实现：

- WebRTCMediaProvider

普通视频字节禁止进入 Runtime Event 持久化或 MCP 模型结果。

## 6. CloudProvider

用途：让任务在本机之外继续执行。

概念能力：

```text
Capabilities
Prepare
UploadSnapshot
Start
Status
Events
FetchArtifacts
Cancel
ResumeLocal
```

潜在实现：

- CodexCloudProvider
- OpenAIHostedProvider
- SelfHostedProvider
- VPSProvider

Core 不假设一定能 SSH、不假设一定是 VM，也不绑定某个 Git 平台。

## 7. SnapshotProvider

用途：创建和恢复可迁移 Workspace Checkpoint。

概念模型：

```text
WorkspaceSnapshot
  id
  project_id
  base_revision
  dirty_patch
  untracked_manifest
  environment_manifest
  runtime_checkpoint
  artifact_refs
```

可能实现：

- Git-based；
- Local content-addressed store；
- Object storage；
- Provider-native cloud snapshot。

Snapshot 应该支持增量生成，在笔记本真正睡眠前就维持一个 Ready Checkpoint。

## 8. ToolProvider

用途：向 Bridge Core 注册可给 AI 使用的工具能力。

例：

- filesystem
- shell
- git
- database
- project custom tool

Tool 必须声明：

- name；
- input/output schema；
- permission verb；
- project/session scope；
- capability requirements。

## 9. SkillProvider

用途：发现并归一化可复用 AI Skill，但不发明 CodeBridge 私有 Skill 格式。

潜在来源：

- Agent Skills / SKILL.md；
- Codex skills；
- OMP project skills；
- MCP-backed skill bundle。

Provider 归一化元数据，但保留原始 Skill 包。

## 10. ControlPlaneProvider

用途：把个人本地控制与未来 Fleet 分离。

实现：

- `LocalControlPlane`：默认；
- `FleetControlPlane`：未来。

Fleet 可负责设备资产、Health、Policy、Version、Audit，但不能重新成为个人调用主链路的必经节点。

## 11. EventSink

所有 Provider 统一向 EventSink 发布生命周期事件，不能各自维护另一套真相。

例：

```text
agent.started
agent.message
computer.observed
computer.action
computer.controller.changed
cloud.handoff.started
artifact.created
provider.health.changed
```

Runtime 负责分配 Run Sequence 并持久化权威 Journal。

## 12. Artifact Contract

Artifact 是与 Provider 无关的结果引用。

类型可包括：

- file
- patch
- commit
- report
- test_result
- screenshot_ref
- recording_ref
- cloud_result
- log_bundle

本地可保存 Artifact 元数据；大对象和敏感 Payload 由 Provider 决定存储方式。

## 13. Capability Discovery

每个 Provider 必须显式返回 Capability，Core 不能只根据 Provider 类型或 OS 猜测。

例：

```text
Computer:
  observe=true
  input=true
  preview=false

Agent:
  durable_session=true
  resume=true
  tool_events=true

Cloud:
  incremental_upload=true
  remote_shell=false
```

这样系统可以安全降级。

## 14. Versioning

Provider Contract 和本地 IPC 都必须版本协商。

至少包括：

- protocol version；
- provider API version；
- feature/capability flags；
- 向后兼容 optional field。

不支持的高风险能力必须明确失败，不能静默降级。

## 15. Failure Semantics

Provider Error 使用稳定分类：

- unavailable
- permission_denied
- unsupported
- invalid_state
- transient
- terminal
- cancelled

是否 Retry / Recovery 由 Core 决定，Provider 不应偷偷维护 Core 无法观察的无限重试循环。

## 16. 架构约束

1. 外部发起的操作必须先经过本机 Permission，Provider 不能绕过。
2. Provider 通过 Runtime 发布 Event，不能形成竞争的生命周期真相。
3. TransportProvider 不能直接调用 OS / Agent / Cloud 实现。
4. ComputerProvider 不负责 ChatGPT 身份认证。
5. 未发生显式 Handoff 时，CloudProvider 不能取代本地 Run 权威状态。
6. Provider 原始数据可本地保留，但 Core 只依赖稳定归一化字段。
