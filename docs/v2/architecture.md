# CodeBridge V2 Architecture

Status: **architecture baseline / intended long-term shape**

## 1. Product definition

CodeBridge is the local control and execution layer between AI clients and a real development environment.

It has three equal architectural pillars:

- **Bridge** — transport, project/workspace access, capability routing, permissions, approvals, credentials, tools and skills.
- **Runtime** — sessions, runs, turns, durable events, artifacts, recovery, scheduling and long-running execution.
- **Computer** — desktop observation, input, preview, controller ownership and future automation.

The architecture is Bridge-first. The roadmap is Computer-first.

## 2. Design goals

V2 must:

- run on the native host by default;
- expose the real development environment rather than a Docker-only approximation;
- keep the public AI-facing surface independent of OMP, Codex, OpenCode, macOS and any one tunnel provider;
- allow local work to survive ChatGPT, tunnel and UI disconnects;
- support Computer Use without creating a second security or lifecycle system;
- support future cloud handoff without binding the core to Codex Cloud;
- reuse CodexBridge implementation where it saves work, while keeping CodeBridge domain contracts independent;
- leave room for Windows, Linux, fleet management, additional AI clients and additional transports.

## 3. System shape

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
|                  |  |                  |  |                  |
| OMP              |  | Observe          |  | provider API     |
| Codex            |  | Input            |  | handoff          |
| OpenCode         |  | Preview          |  | artifacts        |
| future providers |  | Takeover         |  | resume-local     |
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

## 4. Process boundaries

V2 uses two primary local processes.

### 4.1 Native Desktop Service

Preferred implementation: Swift on macOS, reusing CodexBridge native components where practical.

Responsibilities:

- Secure MCP Tunnel integration;
- local MCP entry point;
- desktop UI;
- macOS permissions;
- Keychain/credential integration;
- project registration and approval UI;
- native Computer Provider;
- WebRTC/native media integration;
- updater/packaging;
- lifecycle supervision of the Go runtime.

### 4.2 CodeBridge Runtime

Preferred implementation: existing Go runtime evolved into a sidecar/service.

Responsibilities:

- persistent Session/Run state;
- ordered Event journal;
- Agent Providers;
- orchestration;
- artifact metadata;
- recovery;
- incremental workspace checkpoint metadata;
- future cloud handoff;
- local SQLite runtime state.

The two processes communicate through a versioned local IPC protocol. Internal Swift or Go structs must never become the cross-process contract.

## 5. Core domain model

V2 uses one lifecycle model across agents, computer and cloud.

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

Definitions:

- **Project** — one authorized local workspace or repository context.
- **Session** — a durable working context that may span multiple runs.
- **Run** — one user goal or execution objective.
- **Turn** — one bounded unit of agent/computer/cloud work.
- **Event** — one ordered state change.
- **Artifact** — a durable result or reference: file, patch, commit, report, test result, screenshot reference, cloud result, etc.

Do not introduce independent lifecycle models for Computer, Cloud or individual harnesses.

## 6. Event backbone

All meaningful lifecycle changes produce `RuntimeEvent`.

Conceptual envelope:

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

Sources include:

- bridge
- runtime
- agent
- computer
- cloud
- media
- system

Examples:

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

The local journal is authoritative for recovery. Live delivery is a cache/notification path, never the sole source of truth.

## 7. Provider model

Concrete implementations sit behind provider contracts.

Required provider families:

- `TransportProvider`
- `AgentProvider`
- `ComputerProvider`
- `MediaProvider`
- `CloudProvider`
- `SnapshotProvider`
- `SkillProvider`
- `ToolProvider`
- `ControlPlaneProvider` (future fleet)

Bridge Core must not import or special-case OMP, Codex, OpenCode, macOS, WebRTC, Codex Cloud or CodexBridge internals.

## 8. Bridge Core

Bridge Core is the architectural center. It owns:

- capability discovery and routing;
- project/workspace authorization;
- permission evaluation;
- approval requests;
- session resolution;
- tool/skill registration;
- provider registration;
- transport-independent request handling;
- event publication.

Typical flow:

```text
AI request
  -> TransportProvider
  -> Bridge Core
  -> Permission / Session
  -> Capability Router
  -> Provider
  -> Runtime Event / Artifact
  -> response
```

Computer and Agent operations must not bypass Bridge Core.

## 9. Native-host default

The default personal-developer execution target is the actual host OS.

Therefore local agents can naturally use:

- Docker Desktop;
- brew/package managers;
- launchctl/system services;
- IDEs;
- SSH;
- kubectl;
- VPN/internal network;
- local databases;
- local browser state.

Container/sandbox execution is optional and must be represented as a provider or execution policy, not as the default development-machine architecture.

## 10. Computer model

Computer Use is a first-class capability, not a side channel.

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

Targets are extensible:

- display
- window
- application
- browser
- virtual desktop

V1 only needs the target types it can implement safely.

Controller values:

- agent
- human
- none

Every input operation carries the current `controller_epoch`. Changing controller increments the epoch; queued input from an older epoch is rejected. This is the fundamental human-takeover race barrier.

## 11. Computer data planes

Model observation and human preview are permanently separate.

```text
Computer Runtime
     |
     +-- Observation -> MCP image -> model
     |
     +-- Media -> WebRTC -> human widget
```

Rules:

- model observation uses explicit still images;
- live video never enters model context;
- model actions do not depend on the WebRTC path;
- media failure must not corrupt the ComputerSession;
- screenshots/video are not persisted by default.

## 12. Computer actions

The public model supports two extensibility levels.

### Primitive actions

- move
- click
- double_click
- drag
- scroll
- type
- keypress
- wait

### Automation actions

Reserved for later:

- script
- browser_script
- workflow

Protocol shape must allow additional action kinds without changing the session model.

## 13. Runtime reliability

A started local run is owned by the Runtime, not by a ChatGPT request.

The following must not automatically terminate a run:

- ChatGPT turn ends;
- browser closes;
- Secure MCP Tunnel reconnects;
- desktop UI restarts.

Recovery uses the durable Event journal and provider-native session state when available.

## 14. Cloud handoff

Cloud is a provider capability, not a core dependency.

```text
Local Run
  -> incremental checkpoint
  -> WorkspaceSnapshot
  -> CloudProvider
  -> Cloud Run
  -> Artifact / Diff
  -> Resume Local
```

The core contract must support Codex Cloud, OpenAI-hosted environments, self-hosted environments and future providers.

A workspace snapshot conceptually contains:

- base revision;
- working-tree diff;
- untracked-file manifest;
- environment manifest;
- runtime continuation summary;
- artifact references.

Upload implementation is deferred, but the contract is reserved now.

## 15. Agent runtime and orchestration

OMP, Codex and OpenCode are implementations of `AgentProvider`, not architectural centers.

Future orchestration is provider-independent:

```text
Orchestrator
├── Role
├── TaskGraph
├── Delegation
├── ReviewLoop
├── ModelPolicy
└── BudgetPolicy
```

A role may resolve to any provider/model combination. This enables cross-provider workflows without changing Bridge Core.

## 16. Security model

Permissions use namespaced verbs.

Examples:

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

Effects:

- allow
- deny
- ask

Scopes:

- once
- session
- project
- always

Deny always wins.

Remote callers cannot broaden local permissions.

## 17. Storage boundaries

Local storage may persist:

- project/session/run metadata;
- ordered events;
- provider/session locators;
- artifact metadata;
- terminal results;
- cloud checkpoint metadata;
- local UI indexes.

Do not persist by default:

- screen video;
- screenshots;
- secrets;
- complete source copies solely for telemetry;
- hidden model reasoning.

## 18. Upstream strategy

CodexBridge is an upstream implementation source, not a domain dependency.

Prefer to reuse upstream for:

- native desktop shell;
- Secure MCP Tunnel;
- packaging/updater;
- credential integration;
- approval UI;
- project registration;
- agent discovery.

CodeBridge-specific code should live behind narrow integration hooks such as:

- RuntimeProvider
- ComputerProvider
- EventSink
- UI extension surface

Avoid broad edits across upstream modules.

## 19. Future Fleet

Fleet is explicitly outside the personal V2 critical path.

Reserve `ControlPlaneProvider`:

- `LocalControlPlane` — default;
- `FleetControlPlane` — future.

A future fleet service may add centralized device inventory, policy, audit and health without becoming a mandatory hop in personal MCP/Computer traffic.

## 20. Architectural invariants

1. Bridge Core never depends on a concrete agent, OS, transport, media or cloud provider.
2. Computer, Agent and Cloud capabilities always enter through Bridge Core.
3. Session / Run / Turn / Event / Artifact is the only lifecycle model.
4. Native host is the default personal-developer execution environment.
5. Durable local state is authoritative for recovery.
6. Model observation and human media remain separate planes.
7. CodexBridge is reusable upstream implementation, not CodeBridge's architectural owner.
8. Future capabilities should normally be added by a provider/extension rather than by changing the core model.
