# CodeBridge V2 Provider Contracts

Status: **contract baseline**

The contracts in this document define where future functionality plugs into V2. They are conceptual interfaces; exact language-level signatures may evolve during implementation, but responsibilities and dependency direction should remain stable.

## 1. Dependency rule

```text
Bridge Core
   |
   +--> Provider interface
           ^
           |
     concrete provider
```

Concrete providers may depend on platform SDKs and third-party projects. Bridge Core must not.

## 2. TransportProvider

Purpose: connect an AI client or local caller to Bridge Core.

Responsibilities:

- start/stop transport;
- expose capability/tool metadata;
- deliver authenticated requests;
- stream supported responses/events;
- report transport health.

Planned implementations:

- `SecureMCPTunnelTransport` — default personal remote path;
- `LocalMCPTransport` — local development and testing;
- optional future HTTP/other transports.

Transport must not contain business routing or provider-specific execution logic.

## 3. AgentProvider

Purpose: adapt one local or remote agent harness into the unified Runtime.

Conceptual operations:

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

Planned implementations:

- OMPProvider
- CodexProvider
- OpenCodeProvider
- future OpenAIAgentsProvider
- future custom providers

The provider preserves harness-native history and exposes only stable normalized concepts to Core.

## 4. ComputerProvider

Purpose: expose a controllable computer target.

Conceptual operations:

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

Target types are extensible:

- display
- window
- application
- browser
- virtual

Planned implementation:

- MacComputerProvider first;
- WindowsComputerProvider later;
- LinuxComputerProvider later.

ComputerProvider owns OS interaction, not MCP transport.

## 5. MediaProvider

Purpose: deliver human-facing live media separately from model observation.

Conceptual operations:

```text
StartPreview
CreateViewerGrant
Signal
UpdateQuality
StopPreview
Health
```

Planned implementation:

- WebRTCMediaProvider

Normal video bytes must not pass through Bridge Core event persistence or MCP model results.

## 6. CloudProvider

Purpose: continue work outside the local host.

Conceptual operations:

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

Potential implementations:

- CodexCloudProvider
- OpenAIHostedProvider
- SelfHostedProvider
- future VPSProvider

Core must not assume SSH access, VM semantics or a specific Git host.

## 7. SnapshotProvider

Purpose: create and materialize portable workspace checkpoints.

Conceptual snapshot:

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

Possible implementations:

- Git-backed checkpoint;
- local content-addressed store;
- object-storage-backed snapshot;
- provider-native cloud snapshot.

Snapshot generation should be incremental so a ready checkpoint exists before a laptop sleeps.

## 8. ToolProvider

Purpose: register executable capabilities that Bridge Core can expose to AI clients.

Examples:

- filesystem
- shell
- git
- database
- project-specific custom tools

Tools declare:

- name;
- input/output schema;
- permission verb;
- project/session scope;
- capability requirements.

## 9. SkillProvider

Purpose: discover and normalize reusable AI instructions/skills without inventing a CodeBridge-only skill format.

Potential sources:

- Agent Skills / SKILL.md;
- Codex skills;
- OMP project skills;
- MCP-backed skill bundles.

The provider returns normalized metadata while preserving the source package.

## 10. ControlPlaneProvider

Purpose: abstract personal-local control from future fleet management.

Implementations:

- `LocalControlPlane` — default;
- `FleetControlPlane` — future.

Fleet may manage inventory, health, policy, version and audit. It must not become a required hop for normal personal execution.

## 11. EventSink

Every provider publishes lifecycle events through a common sink rather than owning a separate history mechanism.

Examples:

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

Runtime assigns ordered run sequence numbers and persists the canonical journal.

## 12. Artifact contract

Artifacts are provider-independent references.

Kinds may include:

- file
- patch
- commit
- report
- test_result
- screenshot_ref
- recording_ref
- cloud_result
- log_bundle

Artifact metadata may be persisted locally; large/sensitive payload storage is provider-specific.

## 13. Capability discovery

Every provider returns explicit capabilities. Core must not infer capability solely from provider type or operating system.

Examples:

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

This allows graceful degradation.

## 14. Versioning

Provider contracts and local IPC require version negotiation.

At minimum:

- protocol version;
- provider API version;
- feature/capability flags;
- backward-compatible optional fields.

Unsupported capabilities fail explicitly; they must not silently downgrade high-impact operations.

## 15. Failure semantics

Providers return errors in stable categories:

- unavailable
- permission_denied
- unsupported
- invalid_state
- transient
- terminal
- cancelled

Core decides retry/recovery policy. Providers should not invent independent infinite-retry loops that Core cannot observe.

## 16. Architectural constraints

1. Providers cannot bypass local permission evaluation for externally initiated operations.
2. Providers publish events through Runtime rather than maintaining competing lifecycle truth.
3. TransportProvider never directly invokes OS/agent/cloud implementations.
4. ComputerProvider never owns authentication to ChatGPT.
5. CloudProvider never becomes the local source of truth for a run unless an explicit handoff occurs.
6. Provider-specific raw data may be retained locally, but Core only relies on stable normalized fields.
