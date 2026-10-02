# CodeBridge V2 Design

Status: **ARCHITECTURE FROZEN — 2026-10-02; PHASE_0_BLOCKED; Phase 1 NOT READY**

CodeBridge V2 connects AI clients to the user's real development environment through three first-class domains:

1. **Bridge** — ingress, caller identity, capabilities, project authorization, policy/approval and secrets.
2. **Runtime** — durable sessions, runs, provider sessions, event journal, artifacts and recovery.
3. **Computer** — observation and control of the real desktop with exclusive control and human takeover.

> **Bridge-first architecture, Computer-first roadmap.**

Computer Use is the highest feature priority, and it ships through the same Bridge, Policy and Journal used by files, shell and agents.

## Documents

- [Architecture](architecture.md) — boundaries, process topology, domain model, Computer safety model, security, decision register.
- [Provider Contracts](provider-contracts.md) — frozen, internal and deferred contracts; Host IPC.
- [Roadmap](roadmap.md) — Phase 0 spikes, Computer-first phases.
- [Migration](migration.md) — V1 code disposition, CodexBridge reuse procedure, data migration.
- [Kimi K3 Architecture Audit — 2026-10-02](reviews/2026-10-02-kimi-k3-architecture-audit.md) — independent P0/P1 challenge and remediation record.
- [GLM 5.3 Final Freeze Gate — 2026-10-02](reviews/2026-10-02-glm-5.3-freeze-gate.md) — final gate: `READY_TO_FREEZE`, P0=0, P1=0.
- [Phase 0 implementation closeout](evidence/phase0-closeout.md) — actual checks, per-task results, prerequisites and complete file manifest; code availability is not evidence-gap closure.
- [Phase 0B blocker closure / signed native acceptance](evidence/phase0b-closeout.md) — **PHASE0_BLOCKED**: genuine signing and actual ChatGPT/Tunnel prerequisites absent; F2 not closed, Phase 1 not ready.

Chinese versions:

- [总体架构](architecture.zh-CN.md)
- [Provider 接口](provider-contracts.zh-CN.md)
- [开发路线](roadmap.zh-CN.md)
- [迁移方案](migration.zh-CN.md)

## Key decisions

- **`codebridged` (Go, per-user LaunchAgent) is the single authority**: MCP ingress, Bridge kernel, Policy, Runtime and the durable store.
- **CodeBridge.app (Swift) is the native host adapter**: computer engine and InputArbiter, approval UI, notifications, preview media, updater. The required security invariant is that only this app holds Computer-related macOS TCC grants; Phase 0 must verify that daemon/harness processes are not attributed those grants.
- **Secure MCP Tunnel is OpenAI's `tunnel-client`**, supervised by the daemon; no Manager, no public MCP ingress.
- **Domain model**: Project, Session, Run, ProviderSession, ComputerSession, Event, Artifact. Turn is a correlation field, not an entity.
- **Computer input safety**: every action cites the frame it was planned from; the arbiter validates and injects in one process; physical user input preempts the agent; only a human can resume the agent.
- **No model-visible path can grant permissions.**
- **CodexBridge is an implementation source behind adapters**; audited at `win` / `v1.3.4`, commit `7844bb608a9a4e96ed09c084589b7825db77aa3e` ([upstream evidence](evidence/phase0-upstream.md)). No upstream types or source were imported into CodeBridge's domain, Host IPC or store.

## Architectural rule

Boundaries in the decision register ([Architecture](architecture.md) §19) stay stable while implementations evolve. A new capability is normally a provider implementation or a new tool, not a change to the Bridge kernel or domain model.
