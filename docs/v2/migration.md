# CodeBridge V2 Migration Strategy

Status: **migration baseline**

Current V1 checkpoint before V2 documentation: `d251a3c Build durable runtime and harness session foundation`.

The goal is not to rewrite everything and not to continue V1 unchanged. V2 reuses mature CodexBridge implementation for commodity bridge/desktop concerns while preserving the Runtime work that differentiates CodeBridge.

## 1. Migration principle

```text
Do not rebuild what upstream already solves well.
Do not throw away the durable Runtime we already built.
Do not let upstream internals become CodeBridge's domain model.
```

CodexBridge is an upstream implementation source. CodeBridge owns its own architecture and provider contracts.

## 2. Keep and migrate from current CodeBridge

High-value code/concepts to preserve:

### Runtime

- RunStore;
- RunManager;
- durable ordered RunEvent journal;
- subscriptions;
- reconnect/replay concepts;
- asynchronous run ownership;
- terminal result persistence.

### Harness/session adapters

- HarnessAdapter concept;
- OMP live/session parsing;
- Codex live/session parsing;
- OpenCode session work;
- normalized display projection;
- raw local event retention.

### Computer design

Preserve useful design decisions:

- model still images separate from human live video;
- logical coordinates;
- explicit ComputerSession;
- exclusive controller;
- human takeover;
- WebRTC/TURN separation;
- permission namespaces.

The old Manager/gRPC assumptions around these concepts are not retained as architectural invariants.

## 3. Stop extending on the main path

Freeze rather than delete immediately:

- public Manager MCP ingress;
- Manager as mandatory relay;
- Manager→Client gRPC personal execution path;
- Docker Client as Mac default;
- Caddy/Nginx public MCP routing;
- custom OAuth ingress;
- device enrollment/account routing intended only to support the old relay architecture.

Keep code until V2 proves replacement paths; then archive/remove deliberately.

## 4. Reuse from CodexBridge upstream

Target reuse areas:

- macOS desktop application shell;
- Windows desktop shell later;
- Secure MCP Tunnel integration;
- local service/app lifecycle;
- packaging/signing/updater;
- Keychain/Credential Manager integration;
- project registration;
- approval UI;
- agent discovery;
- existing agent connectivity where useful;
- existing workspace security mechanisms where compatible.

Before importing/forking:

- audit module boundaries;
- audit licenses and NOTICE obligations;
- identify minimal extension points;
- build upstream unchanged;
- run upstream tests/smokes;
- record upstream commit/tag used as baseline.

## 5. Thin-upstream rule

Avoid a fork that edits upstream everywhere.

Preferred shape:

```text
upstream CodexBridge
       |
       +-- narrow CodeBridge hooks
               |
               +-- Runtime adapter
               +-- Computer provider
               +-- Event sink
               +-- UI extensions
```

Upstream sync should touch a small number of integration files.

If a feature can be implemented outside upstream core, keep it outside.

## 6. Repository target shape

Conceptually:

```text
codebridge/
├── upstream/                 # optional vendored/fork boundary
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
├── runtime/                  # Go engine
├── desktop/                  # native integration/extensions
└── docs/v2/
```

The exact physical layout may follow upstream build constraints. Dependency direction matters more than folder names.

## 7. Migration stages

### Stage A — Baseline

1. freeze current V1 checkpoint;
2. create V2 docs/contracts;
3. audit CodexBridge upstream;
4. build upstream unchanged;
5. smoke Tunnel, project, shell and Docker on native Mac.

### Stage B — Integration seam

1. define RuntimeProtocol v1;
2. define provider registration;
3. define EventSink;
4. define IDs/schema versions;
5. add minimal upstream hooks;
6. prove Native Service ↔ Go Runtime health handshake.

### Stage C — Computer-first implementation

1. MacComputerProvider;
2. observe;
3. input;
4. ComputerSession;
5. permissions;
6. real ChatGPT E2E;
7. WebRTC/takeover.

### Stage D — Runtime migration

1. port RunStore/Event journal;
2. map old Run model into V2 Session/Run/Turn/Event;
3. port Agent providers/adapters;
4. add durable recovery;
5. add Runtime UI/notification.

### Stage E — orchestration/cloud

Only after Computer + Runtime are stable.

## 8. Data migration

Do not silently reinterpret old databases.

Use explicit schema versioning.

Options:

- read old V1 `runs.db` and migrate once;
- keep old DB read-only and start V2 DB;
- export/import selected run history.

Decision is deferred until V2 storage schema is finalized.

## 9. Configuration migration

Translate old concepts instead of preserving old deployment assumptions.

Examples:

```text
CODEBRIDGE_MANAGER_HOST
    -> removed from default personal configuration

workspaces
    -> Projects / Workspace authorization

permissions
    -> namespaced Permission rules

subagent_profiles
    -> AgentProvider profiles / ModelPolicy
```

Do not automatically copy secrets between storage systems.

## 10. Testing strategy

Maintain three levels:

1. **Core contract tests** — no real OS/provider.
2. **Provider tests** — fixture/fake + targeted real local smoke.
3. **End-to-end** — ChatGPT/Tunnel/native host for critical flows.

Highest-priority real E2E:

```text
ChatGPT
 -> Secure MCP Tunnel
 -> Native CodeBridge
 -> computer_observe
 -> computer_action
 -> computer_observe
```

Second:

```text
ChatGPT
 -> agent_start
 -> local durable run
 -> disconnect
 -> continue locally
 -> reconnect
 -> replay/final result
```

## 11. Commit strategy

Keep migration changes reviewable:

- baseline/upstream import;
- extension contracts;
- native bridge integration;
- Computer MVP;
- Runtime migration;
- orchestration;
- cloud.

Do not combine upstream synchronization with large CodeBridge feature changes in the same commit.

## 12. Rollback rule

Until V2 reaches native Computer E2E:

- V1 remains buildable from the frozen checkpoint;
- no destructive deletion of Manager/Client code;
- V2 changes should be reversible by branch/commit;
- upstream syncs are isolated.

After V2 proves the replacement path, old deployment components can be archived or removed in a dedicated cleanup milestone.
