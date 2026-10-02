# CodeBridge V2 Design

Status: **architecture baseline / pre-implementation**

CodeBridge V2 is designed around three equal architectural pillars:

1. **Bridge** — securely connects AI clients to the user's real development environment.
2. **Runtime** — makes local work durable, asynchronous, recoverable and observable.
3. **Computer** — lets AI observe and operate the real desktop with explicit local permission and human takeover.

The implementation roadmap is intentionally different from the architectural importance:

> **Bridge-first architecture, Computer-first roadmap.**

Computer Use is the highest new-feature priority, but it must be implemented through the same Bridge, Session, Permission, Event and Runtime contracts used by agents, shell tools and future cloud execution.

## Documents

- [Architecture](architecture.md) — long-term V2 system boundaries and invariants.
- [Provider Contracts](provider-contracts.md) — extension points for transport, agents, computer, media, cloud, snapshots and future fleet control.
- [Roadmap](roadmap.md) — implementation order; Computer Use is the first major feature.
- [Migration](migration.md) — how to reuse CodexBridge while preserving valuable CodeBridge Runtime work.

Chinese versions:

- [总体架构](architecture.zh-CN.md)
- [Provider 接口](provider-contracts.zh-CN.md)
- [开发路线](roadmap.zh-CN.md)
- [迁移方案](migration.zh-CN.md)

## Architectural rule

The V2 architecture is intended to stay stable while implementations evolve. New capabilities should normally be added by implementing a provider or extension contract rather than changing Bridge Core.

CodexBridge is treated as an **upstream implementation source** for native desktop, Secure MCP Tunnel integration, packaging, approval UI, credential storage and agent discovery. It is not an architectural dependency of CodeBridge Core.
