# CodeBridge V2 设计

状态：**ARCHITECTURE FROZEN —— 2026-10-02；PHASE_0_BLOCKED；Phase 1 NOT READY**

CodeBridge V2 通过三个一等领域（domain）把 AI client 连接到用户的真实开发环境：

1. **Bridge** —— 入口（ingress）、调用方身份、能力、项目授权、policy/approval 与密钥。
2. **Runtime** —— 持久化的 session、run、provider session、事件 Journal、artifact 与恢复。
3. **Computer** —— 对真实桌面的观察与控制，具备独占控制权与人工接管。

> **Bridge-first architecture，Computer-first roadmap。**

Computer Use 是最高功能优先级，并且它与 files、shell 和 agents 使用同一套 Bridge、Policy 与 Journal。

## 文档

- [总体架构](architecture.zh-CN.md) —— 边界、进程拓扑、领域模型、Computer 安全模型、安全、决策登记。
- [Provider 接口](provider-contracts.zh-CN.md) —— 冻结（Frozen）、内部与延后（Deferred）的接口；Host IPC。
- [开发路线](roadmap.zh-CN.md) —— Phase 0 技术验证（spike）、Computer-first 的各阶段。
- [迁移方案](migration.zh-CN.md) —— V1 代码处置、CodexBridge 复用流程、数据迁移。
- [Kimi K3 架构审查记录 — 2026-10-02](reviews/2026-10-02-kimi-k3-architecture-audit.zh-CN.md) —— 独立 P0/P1 Challenge 与整改记录。
- [GLM 5.3 最终 Freeze Gate — 2026-10-02](reviews/2026-10-02-glm-5.3-freeze-gate.zh-CN.md) —— 最终 Gate：`READY_TO_FREEZE`，P0=0，P1=0。
- [Phase 0 实施收尾](evidence/phase0-closeout.zh-CN.md) —— 实际检查、逐项结果、缺失前提与完整文件清单；代码可运行不等于 Evidence Gap 已关闭。
- [Phase 0B 阻塞关闭 / 真实签名原生验收](evidence/phase0b-closeout.zh-CN.md) —— **PHASE0_BLOCKED**：真实签名与 ChatGPT/Tunnel 外部前提缺失；F2 未关闭，Phase 1 未就绪。

英文版：

- [Architecture](architecture.md)
- [Provider Contracts](provider-contracts.md)
- [Roadmap](roadmap.md)
- [Migration](migration.md)

## 关键决策

- **`codebridged`（Go，per-user LaunchAgent）是唯一权威**：MCP 入口（ingress）、Bridge 内核、Policy、Runtime 与持久化 store。
- **CodeBridge.app（Swift）是原生宿主适配层**：computer engine 与 InputArbiter、审批 UI、通知、preview 媒体、updater。安全硬要求是只有该 App 持有 Computer 相关 macOS TCC 授权；Phase 0 必须实测 daemon / harness 进程不会被归属到这些授权。
- **Secure MCP Tunnel 是 OpenAI 的 `tunnel-client`**，由 daemon 托管；没有 Manager，也没有公网 MCP 入口（ingress）。
- **领域模型**：Project、Session、Run、ProviderSession、ComputerSession、Event、Artifact。Turn 是一个关联字段，而不是实体。
- **Computer 输入安全**：每个 action 都引用它规划时所用的帧（frame）；arbiter 在同一进程内完成校验与注入；用户的物理输入会抢占 agent；只有人才能恢复 agent。
- **任何模型可见的路径都不能授予权限。**
- **CodexBridge 是位于 adapter 之后的实现来源**；已审计 `win` / `v1.3.4`，commit `7844bb608a9a4e96ed09c084589b7825db77aa3e`（[上游证据](evidence/phase0-upstream.md)）。未将上游类型或源码引入 CodeBridge domain、Host IPC 或 store。

## 架构规则

决策登记（[总体架构](architecture.zh-CN.md) §19）中的边界在实现演进过程中保持稳定。新增能力通常是一个 provider 实现或一个新工具，而不是对 Bridge 内核或领域模型的修改。
