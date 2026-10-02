# CodeBridge V2 Roadmap

Status: **implementation roadmap**

The architecture is Bridge-first; the implementation roadmap is Computer-first.

This roadmap deliberately separates a short architecture/baseline prerequisite from feature priority. **Computer Use is the first major V2 feature.**

## Phase 0 — Architecture freeze and upstream baseline

Goal: establish stable boundaries before adding new feature code.

- [x] Define V2 architecture baseline.
- [x] Define Provider contracts.
- [x] Define migration strategy.
- [ ] Audit current CodexBridge upstream structure and extension points.
- [ ] Confirm reuse/licensing/NOTICE obligations.
- [ ] Build the upstream macOS application unchanged.
- [ ] Run Secure MCP Tunnel smoke.
- [ ] Run native workspace/shell/Docker smoke.
- [ ] Define versioned Native Service ↔ Go Runtime IPC.
- [ ] Define shared IDs and schema versioning for Project/Session/Run/Event/Artifact.

Exit condition:

- CodeBridge can reuse upstream without broad invasive patches.
- A native macOS process can receive a Bridge request and run a real host command.

## Phase 1 — Computer Use MVP (**highest feature priority**)

Goal: ChatGPT can safely observe and control the real Mac.

### Computer foundation

- [ ] Implement MacComputerProvider.
- [ ] Detect Screen Recording permission.
- [ ] Detect Accessibility permission.
- [ ] Enumerate displays.
- [ ] Reserve window/application target model.
- [ ] Normalize Retina/logical coordinates.
- [ ] Add frame IDs and display-change invalidation.

### Observation

- [ ] `computer_start`
- [ ] `computer_observe`
- [ ] still-image capture through ScreenCaptureKit
- [ ] image dimension/byte bounds
- [ ] permission enforcement
- [ ] explicit unchanged-frame semantics

### Input

- [ ] move
- [ ] click
- [ ] double click
- [ ] drag
- [ ] scroll
- [ ] type
- [ ] keypress
- [ ] wait
- [ ] ordered batches stop on first failure
- [ ] controller_epoch validation

### Lifecycle

- [ ] `computer_status`
- [ ] `computer_stop`
- [ ] lease/expiry
- [ ] local approval integration
- [ ] metadata-only events

### Required E2E

- [ ] ChatGPT → Secure MCP Tunnel → CodeBridge → observe Mac
- [ ] act → observe
- [ ] permission denied path
- [ ] display resolution change path
- [ ] stale controller epoch rejection

Exit condition:

> A real ChatGPT conversation can observe, click, type and scroll on the user's Mac without Manager, Docker Client or public MCP infrastructure.

## Phase 2 — Live Computer and Human Takeover

Goal: the user can watch and safely take control.

- [ ] MediaProvider baseline.
- [ ] WebRTCMediaProvider.
- [ ] signaling service with short-lived grants.
- [ ] STUN/direct path.
- [ ] TURN fallback.
- [ ] adaptive FPS/resolution/bitrate.
- [ ] ChatGPT Computer widget.
- [ ] Pause.
- [ ] Take Over.
- [ ] input barrier.
- [ ] increment `controller_epoch`.
- [ ] Human control.
- [ ] Resume Agent.
- [ ] mandatory fresh observation after resume.
- [ ] loss-of-human-link returns controller to `none`, never silently to agent.

Exit condition:

> Human and agent can never inject input concurrently, and preview failure does not corrupt model control.

## Phase 3 — Runtime V2 migration

Goal: carry forward the valuable current Runtime foundation into the new native architecture.

Migrate/reuse from the current Go codebase:

- [ ] RunStore.
- [ ] ordered RunEvent journal.
- [ ] subscriptions.
- [ ] RunManager.
- [ ] OMP session adapter.
- [ ] Codex session adapter.
- [ ] OpenCode adapter.
- [ ] recovery/replay.
- [ ] terminal result persistence.

Refactor to the V2 model:

- [ ] Project.
- [ ] Session.
- [ ] Run.
- [ ] Turn.
- [ ] RuntimeEvent.
- [ ] Artifact.
- [ ] AgentContext.
- [ ] ComputerContext.
- [ ] CloudContext.

Add:

- [ ] Runtime UI.
- [ ] Conversation/Activity/Artifacts/Computer/Raw views.
- [ ] asynchronous completion notification.
- [ ] provider health.

Exit condition:

> Long work continues independently of ChatGPT/Tunnel/UI lifetime and reconnects from the durable local journal.

## Phase 4 — OMP and cross-provider orchestration

Goal: turn CodeBridge from an agent gateway into a provider-independent local agent runtime.

- [ ] Orchestrator interface.
- [ ] project-level main agent.
- [ ] Role model.
- [ ] planner/coder/reviewer/operator roles.
- [ ] Delegation.
- [ ] TaskGraph.
- [ ] ReviewLoop.
- [ ] ModelPolicy.
- [ ] BudgetPolicy.
- [ ] cross-provider Event correlation.
- [ ] OMP-first implementation.
- [ ] preserve Codex/OpenCode as independent providers.
- [ ] evaluate OpenAI Agents provider instead of duplicating its multi-agent protocol.

Exit condition:

> A project can route different roles to different provider/model combinations without Bridge Core changes.

## Phase 5 — Local ↔ Cloud handoff

Goal: local work can continue when the laptop sleeps without turning CodeBridge into a cloud platform.

### Contracts first

- [ ] WorkspaceSnapshot.
- [ ] SnapshotProvider.
- [ ] CloudProvider.
- [ ] EnvironmentManifest.
- [ ] RunCheckpoint.
- [ ] Artifact return contract.

### Incremental readiness

- [ ] base revision tracking.
- [ ] dirty patch.
- [ ] untracked manifest.
- [ ] environment manifest.
- [ ] ready checkpoint while laptop is awake.

### Initial cloud provider

- [ ] evaluate Codex Cloud handoff path.
- [ ] start cloud continuation.
- [ ] retrieve diff/artifacts.
- [ ] reconcile return to local.
- [ ] explicit conflicts; never overwrite local changes silently.

Later providers:

- OpenAI hosted.
- self-hosted.
- user VPS.

Exit condition:

> A run can explicitly hand off to one supported cloud provider and return artifacts/diffs safely.

## Phase 6 — Additional platforms

- [ ] Windows native Bridge.
- [ ] Windows ComputerProvider.
- [ ] Linux native Bridge.
- [ ] Linux X11/Wayland ComputerProvider.
- [ ] platform-specific permission models.

## Phase 7 — Optional sandbox execution

- [ ] Sandbox execution policy/provider.
- [ ] Docker/VM isolation.
- [ ] explicit capability differences from native host.
- [ ] never make sandbox the default personal developer environment.

## Phase 8 — Fleet / enterprise control plane

Only start after demonstrated demand.

- [ ] FleetControlPlane.
- [ ] inventory.
- [ ] centralized policy.
- [ ] version management.
- [ ] health.
- [ ] audit metadata.

Fleet must remain optional and outside the default personal request data path.

## Work explicitly stopped from the mainline

Do not spend V2 critical-path time on:

- public Manager MCP ingress;
- mandatory VPS;
- Caddy/Nginx MCP routing;
- self-built OAuth ingress;
- Docker Client as the default Mac execution environment;
- Manager→Client gRPC routing as the personal execution path;
- duplicating mature CodexBridge desktop/updater/credential features;
- adding agent adapters only to compete on adapter count.

## Priority rule

When priorities conflict:

1. architecture/security correctness;
2. Computer Use;
3. native Bridge reliability;
4. Runtime durability;
5. orchestration;
6. cloud handoff;
7. additional platforms;
8. Fleet.
