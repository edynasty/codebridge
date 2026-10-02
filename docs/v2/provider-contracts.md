# CodeBridge V2 Provider Contracts

Status: **contract baseline — revised by the 2026-10-02 architecture review**

This document defines where functionality plugs into V2, which contracts are frozen now, and which are intentionally left undefined. Exact language-level signatures are written in Phase 0/1; responsibilities, dependency direction and the rules below are binding.

## 1. Contract tiers

| Tier | Meaning | Contracts |
| --- | --- | --- |
| **Frozen v1** | only additive changes inside V2 | AgentProvider; ComputerProvider (Host IPC `computer` service); Host IPC envelope and handshake; capability descriptor; event emission; artifact; error model |
| **Internal port** | Go interfaces inside `codebridged`; may change freely | ingress adapters, SecretStore, ApprovalPresenter, Notifier, TunnelSupervisor |
| **Deferred** | named, intentionally undefined; only binding constraints | CloudProvider, workspace checkpoint, standalone MediaProvider, SkillProvider, ControlPlane / Fleet, sandbox execution |

Why fewer frozen families than the previous nine: only AgentProvider and ComputerProvider have an implementation today or in Phase 1. Freezing an interface without an implementation freezes guesses — for example, an upload-a-snapshot CloudProvider does not match Codex Cloud's GitHub-based environments, and a TransportProvider duplicates what `tunnel-client` and MCP transports already are.

## 2. Dependency rules

```text
Bridge kernel ──> domain services ──> provider ports <── provider implementations
                                      (Go interface or Host IPC schema)
```

1. A provider imports only its port, the event-emission contract and the error model.
2. **No provider calls another provider.** Composition belongs to domain services (for example Runtime coordinating checkpoint and cloud) or to Bridge requests (an agent that needs the computer calls CodeBridge tools as `agent_internal`).
3. Out-of-process providers are reached only through versioned Host IPC. The IPC schema is language-neutral and lives in the repository; it is never generated from Swift or Go structs.
4. Providers never decide policy. They report capabilities and autonomy so Policy can decide.
5. Providers never persist CodeBridge lifecycle truth. They emit events; Runtime commits them.
6. The Bridge kernel and domain services never import a concrete provider, platform SDK or upstream module.

## 3. Common provider elements

Every provider exposes:

- **Describe** — provider id, implementation version, contract version, explicit capability flags, limits;
- **Health** — `ok | degraded | unavailable` with a reason;
- **Errors** — the categories in §11;
- **Cancellation** — honored promptly;
- **Idempotency key** — accepted on mutating operations that a caller may retry.

## 4. Capability descriptor (frozen v1)

Every tool the Bridge exposes declares:

- **name** — stable; a breaking change takes a new name;
- **input and output schema** — output schemas are mandatory (V1 already declares them in `internal/manager/tool_output.go`);
- **permission verb and selector extractor** — how the resource is derived from the arguments;
- **risk class** — `read | write | execute | control | egress`;
- **allowed caller classes** — for example UI-only tools;
- **scope requirements** — project, session or computer session;
- **result visibility** — model-visible content versus widget-only `_meta`;
- **idempotency** — safe to retry, or requires a key.

Tools are registered by domain services into the Bridge kernel's capability registry. There is no separate ToolProvider family.

## 5. AgentProvider (frozen v1)

Purpose: adapt one agent harness into Runtime Runs.

```text
Describe        -> capabilities {resume, history, live_events, cancel, reattach, approval_routing},
                   autonomy modes, models
Start(run_id, project root, task, model, role?, autonomy, provider_session_ref?)
                -> Execution
Execution
  Events        stream of ProviderEvent {normalized kind?, turn_ref?, raw (local only),
                                         provider_session_ref when bound}
  Wait          -> Outcome {status, final output, error category}
  Cancel
  Process       -> process identity {pid, start time, process group}
Reattach(provider_session_ref, process identity) -> Execution      [optional: reattach]
ListSessions / ReadSession(ref, cursor)                             [optional: history]
```

Rules:

- **Bind early.** The provider binds the provider-native session id as soon as live output reveals it. Time/task heuristics are a fallback and must be marked as such.
- **Resume** is `Start` with `provider_session_ref`. Runtime guarantees at most one active Run per ProviderSession.
- **Normalized vs. raw.** Core relies only on normalized kinds (`message`, `tool_call`, `tool_result`, `file_change`, `command`, `error`, `runtime`). Raw records stay local.
- **Truthful autonomy.** The provider declares what each autonomy mode means on this harness (for example OMP `--auto-approve` = full host access; Codex `--full-auto` = Codex's own sandboxed workspace-write mode) and never runs more permissively than the granted mode.
- **Process hygiene.** Harnesses run in their own process group; process identity is reported so recovery can find orphans.
- **Integration mechanism is private.** CLI `exec`, Codex `app-server` JSON-RPC or an SDK is a provider-internal choice that never changes this contract.

V1 mapping: the launch code in `internal/agentops/subagent.go` and the `HarnessSessionAdapter` implementations (`List` / `Discover` / `Read` / `NormalizeLive`) merge into one AgentProvider per harness.

Implementations: OMP, Codex and OpenCode (Phase 3); others later.

## 6. ComputerProvider (frozen v1)

Purpose: expose computer targets behind an authoritative input arbiter. On macOS this is the Host IPC `computer` service implemented by CodeBridge.app.

```text
describe          -> platform, capabilities {observe, input, preview, ax_tree, targets[]},
                     permission states (screen recording, accessibility, input monitoring)
targets           -> displays (Phase 1); windows and applications as filters / scope
open_session(cmp, target, grants {verbs, app selectors}, lease)        -> session state
observe(cmp)      -> frame {frame_id, image, geometry, arbiter_instance,
                            controller_epoch, geometry_generation}
act(cmp, frame_id, actions[], observe_after)  -> per-action results, optional frame
control(cmp, op, actor)                       -> controller state
    op ∈ acquire_agent | pause | takeover | resume_agent | release
close_session(cmp)
notifications     -> controller.changed, preempted, permission.changed,
                     display.changed, session.state
```

Preview (Phase 2) is an optional capability of the same service: `preview.start`, `preview.signal`, `preview.stop`, with remote human input over the preview data channel.

Every implementation must:

- run the arbiter and the injector in the same process, with a single serialized injector;
- stamp and validate frames as specified in [Architecture](architecture.md) §11.5;
- implement local-input preemption, protected surfaces and held-input release;
- accept `resume_agent` (leaving a human hold) only from a human actor — the local UI or a remote control grant — never from the model path;
- fail closed on unknown state; a restart is a new `arbiter_instance` with `controller = none`;
- never persist frames.

Implementations: MacComputerProvider (Swift, in CodeBridge.app) in Phase 1; Windows and Linux later, in any language, behind the same IPC contract.

## 7. Host IPC v1 (frozen envelope)

- **Transport:** Unix domain socket (named pipe on Windows) in a `0700` per-user directory. `codebridged` listens; CodeBridge.app connects.
- **Framing:** length-prefixed JSON-RPC 2.0, bidirectional, with binary attachment frames for images. Control messages stay JSON.
- **Handshake:** `host.hello {protocol: {major, minor}, app_version, role, capabilities[]}` → `{accepted, protocol, daemon_version}`.
- **Services:** `host` (hello, health, prepare_restart), `computer` (§6), `approval` (present, decision), `notify` (post), `runtime` (read models by cursor and user commands, issued as `local_ui` caller).
- **Compatibility:** additive minor changes; N and N-1 minor versions interoperate; unknown fields are ignored; unknown methods return `unsupported`; a major mismatch disables the app's services.
- **Authentication:** peer UID check; code-signature verification of a peer claiming the app role.
- **Schema location:** language-neutral schema in the repository (Phase 0 decides the format); Go and Swift bindings are generated from it or hand-written against it.

## 8. Internal ports

These are Go interfaces inside `codebridged`, not public extension points:

- **Ingress adapters** — remote MCP through `tunnel-client` to Streamable HTTP over the daemon's Unix-domain socket; optional local adapters; Host IPC for `local_ui`.
- **SecretStore** — Keychain on macOS.
- **ApprovalPresenter** — CodeBridge.app over Host IPC; the widget for `remote_human` approvals.
- **Notifier** — local notifications through the app.
- **TunnelSupervisor** — starts, monitors and restarts `tunnel-client`.

## 9. Event emission (frozen v1)

Providers emit events; Runtime assigns `stream`, `seq` and `pos` and commits them. Out-of-process providers emit through IPC notifications, which the daemon journals. No provider maintains a competing history. The envelope is defined in [Architecture](architecture.md) §8.

## 10. Artifact contract (frozen v1)

Kinds: `file`, `patch`, `commit`, `branch_ref`, `report`, `test_result`, `log_bundle`, `cloud_result`, `saved_screenshot` (only after an explicit user action).

Fields: id, producer (run or computer session), kind, reference, digest, size, sensitivity, retention class. Large payloads live in the local artifact directory or behind an external reference.

## 11. Failure semantics (frozen v1)

| Category | Meaning |
| --- | --- |
| `unavailable` | provider or host capability not reachable |
| `permission_denied` | policy or OS permission denies the operation; stable reasons include `host_permission_required` for missing host/TCC access |
| `approval_required` | policy says `ask`; carries an approval id, never an approval token, in model-visible output |
| `unsupported` | capability or action kind not offered |
| `invalid_state` | operation not valid now; carries a reason: `controller_changed`, `stale_frame`, `geometry_changed`, `lease_expired`, `protected_surface`, `out_of_scope`, … |
| `conflict` | result cannot be applied cleanly (for example a cloud diff) |
| `transient` | retry may succeed |
| `terminal` | retry will not succeed |
| `cancelled` | cancelled by caller, user or policy |

Core decides retry and recovery. Providers do not run hidden infinite retry loops.

## 12. Capability discovery

Every provider returns explicit capability flags. Core never infers capability from provider type or operating system, because permissions can disable capture or input at runtime.

```text
Computer:  observe=true  input=true  preview=false  ax_tree=false  input_monitor=true
Agent:     resume=true   history=true  reattach=false  approval_routing=false
```

Unsupported high-impact capabilities fail explicitly; they never silently downgrade.

## 13. Versioning

- Provider contracts carry a contract version.
- Host IPC negotiates `major.minor` and capability flags (§7).
- The store carries `schema_version` with forward-only migrations.
- Optional fields are backward compatible; required-field changes need a new major version.

## 14. Deferred contracts and their binding constraints

### 14.1 CloudProvider

Not defined until Phase 5. Binding now:

- continuation is a new Run with `continues_run_id`, never a migrated run;
- no SSH, VM or uploaded-environment assumption;
- results return as artifacts and are applied locally by explicit action;
- upload is separately permissioned;
- Codex Cloud is GitHub-repository based: handoff means pushing a checkpoint branch, submitting a task and fetching a diff.

### 14.2 Workspace checkpoint (formerly SnapshotProvider)

A Runtime/Project internal service, git-based (base commit + WIP commit on a private ref, allowlisted untracked files, sensitive paths excluded). It becomes a provider only if a real cloud target requires a second checkpoint format.

### 14.3 MediaProvider

Phase 2 preview is a capability of ComputerProvider, because video comes from the same capture pipeline and human input must reach the same arbiter. A standalone MediaProvider is defined only when a second media source exists.

### 14.4 SkillProvider

Removed from the baseline. Skills belong to agent harnesses.

### 14.5 ControlPlane / Fleet

Removed. A future fleet is an optional outbound connector; managed policy can only restrict.

### 14.6 Sandbox execution

Phase 7. Expressed as an autonomy/environment capability of AgentProvider and of shell tools (container or VM), never as the default.

### 14.7 TransportProvider

Removed. MCP transports and `tunnel-client` already are the transport; CodeBridge needs only ingress adapters (§8). A non-MCP transport would be a new ingress adapter, not a provider family.

## 15. Architectural constraints

1. Providers cannot bypass Policy for externally initiated operations.
2. Providers publish events through Runtime and never hold competing lifecycle truth.
3. Ingress adapters never call providers directly.
4. ComputerProvider never authenticates to ChatGPT and never decides policy; it enforces controller, frame and scope rules.
5. A cloud provider never becomes the local source of truth for a run; it returns artifacts.
6. Provider raw data may be retained locally; Core relies only on normalized fields.
7. Upstream-derived code implements CodeBridge ports through adapters; it never defines them.
