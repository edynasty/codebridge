# CodeBridge V2 设计文档

状态：**架构基线 / 实现前设计**

CodeBridge V2 围绕三个地位相同的架构支柱设计：

1. **Bridge**：安全连接 AI Client 与真实开发环境。
2. **Runtime**：让本地任务持久、异步、可恢复、可观测。
3. **Computer**：让 AI 在本机权限约束下观察和操作真实桌面，并支持 Human Takeover。

实现优先级与架构地位刻意分开：

> **Bridge-first architecture，Computer-first roadmap。**

Computer Use 是当前最高的新功能优先级，但必须复用 Bridge、Session、Permission、Event、Runtime，而不是建立第二套旁路体系。

## 文档

- [总体架构](architecture.zh-CN.md)
- [Provider 接口](provider-contracts.zh-CN.md)
- [开发路线](roadmap.zh-CN.md)
- [迁移方案](migration.zh-CN.md)

英文版：

- [Architecture](architecture.md)
- [Provider Contracts](provider-contracts.md)
- [Roadmap](roadmap.md)
- [Migration](migration.md)

## 架构规则

V2 的长期边界应保持稳定，具体实现可以持续替换。新增能力原则上通过 Provider / Extension 接入，而不是修改 Bridge Core。

CodexBridge 被视为 Native Desktop、Secure MCP Tunnel、打包、审批、凭证和 Agent Discovery 等能力的**上游实现来源**，而不是 CodeBridge Core 的架构依赖。
