# CodeBridge V2 Architecture

Status: **ARCHITECTURE FROZEN — 2026-10-02**

Code baseline: `d251a3c Build durable runtime and harness session foundation`. Document baseline: `8bcc5ba Define CodeBridge V2 architecture and migration plan`.

For V2, this document supersedes conflicting statements in [`docs/runtime.md`](../runtime.md) and [`docs/computer-use.md`](../computer-use.md). Those files remain the V1 design record; in particular their Manager/gRPC control plane and Manager-owned signaling are not part of V2.

## 1. Product definition

CodeBridge connects AI clients to the user's real development environment. It is a **Bridge**, a **Persistent Runtime** and a **Computer control layer**.

Three first-class domains:

- **Bridge** — how requests enter, who is calling and what they may do: ingress, caller identity, capability registry, project authorization, policy/approval and secrets.
- **Runtime** — durable ownership of work: sessions, runs, provider sessions, the event journal, artifacts, recovery and notification.
- **Computer** — the real desktop as a controlled resource: observation, input, exclusive control, live preview and human takeover.

> **Bridge-first architecture, Computer-first roadmap.**

The Bridge is the long-term center of the architecture. Computer Use is the highest current feature priority. Runtime provides reliable lifecycle for agents, computer sessions and future cloud continuation.

CodeBridge V2 is **not**:

- **a Computer app** — the native macOS app is a host adapter; authority stays in the Bridge/Runtime daemon;
- **a CodexBridge plugin** — upstream code is consumed behind CodeBridge-owned adapters; CodeBridge never registers its domain into an upstream tool registry or session model;
- **a cloud platform** — cloud is a continuation target, not the system of record;
- **a fleet manager** — the personal path has no mandatory central service.

### 1.1 Positioning against first-party Computer Use

Observed 2026-10-02: the ChatGPT desktop app ships a first-party Computer Use plugin for macOS and Windows with per-app approval ([Codex Computer Use docs](https://developers.openai.com/codex/computer-use)). "The model can click my Mac" is therefore not a differentiator by itself.

CodeBridge Computer Use is justified only by properties the first-party feature does not combine:

- reaching the machine from any ChatGPT surface through Secure MCP Tunnel, not only from a desktop app running on the same machine;
- one Session / Policy / Journal shared with files, shell and local agents;
- one ComputerSession contract usable by non-OpenAI agent providers;
- live preview and human takeover inside the ChatGPT widget.

The Phase 1 exit review must re-check this positioning.

## 2. Design goals

V2 must:

- run on the native host by default;
- expose the real development environment rather than a Docker approximation;
- keep the AI-facing surface independent of OMP, Codex, OpenCode, macOS and any single tunnel;
- keep local work alive across ChatGPT, tunnel and UI disconnects without model polling;
- deliver Computer Use through the same Bridge, Policy and Journal used by every other capability;
- guarantee that agent input is never injected while a human controls or touches the machine;
- leave a correct seam for cloud continuation without binding to Codex Cloud;
- reuse CodexBridge implementation where it saves work, without letting it own CodeBridge's domain;
- keep room for Windows, Linux, more AI clients and an optional future fleet.

## 3. Evidence base

The review judged the design against code and external facts, not against the V2 documents alone.

### 3.1 Verified in this repository (`d251a3c`)

- `internal/agentops` contains a working `RunManager` (state machine `queued → running → completed | failed | cancelled | timeout | interrupted`, concurrency limit, cancellation, timeouts), a SQLite `runStore` and a per-run `RunEvent` journal with `SubscribeEvents` / `EventsAfter`. On restart, non-terminal runs become `interrupted`; there is no reattach or resume.
- `agentops` imports only `internal/protocol` and `modernc.org/sqlite`. gRPC/Manager coupling lives only in `cmd/client` (`run_event` / `run_heads` / `run_replay`) and `internal/manager` (`RunEventBroker`, registry, OAuth).
- Harnesses are launched as CLI subprocesses with their own auto-approval flags (`omp --auto-approve`, `codex exec --full-auto`, `opencode run --auto`).
- `HarnessSessionAdapter` implementations for OMP, Codex and OpenCode exist and are tested but are **not wired** into runs; runs do not record a provider-native session id.
- There is no Project, Session, Turn or Artifact entity in code.
- The run database lives under `os.UserCacheDir()` (`~/Library/Caches/codebridge` on macOS), a location the OS may purge.
- Run rows and run events are written in separate statements, not one transaction.
- V1 `permission_grant` is a model-callable MCP tool, declared read-only, that can persist an `always` allow rule. The model can approve its own permission requests.
- Computer Use has no code; only design documents.
- CodexBridge has no code or vendored copy in this repository. The intended upstream has now been externally identified as `Fanch-hui/codex-bridge` (default development branch `win`, Apache-2.0); CodeBridge has not yet pinned a commit or built that upstream locally.

#### 3.1.1 Phase 0 implementation update

The baseline above remains a record of V1 at `d251a3c`. Phase 0 adds an independent Go daemon, a minimal Swift app/probe, language-neutral Host IPC/capability/store contracts and an Application Support SQLite WAL store. V1 remains unchanged. [Implementation closeout](evidence/phase0-closeout.md) records **PHASE_0_BLOCKED**: signed TCC isolation, protected-root grant persistence and real ChatGPT/Tunnel approval evidence are not established.

### 3.2 Verified externally (2026-10-02)

- Secure MCP Tunnel is OpenAI's [`tunnel-client`](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels): it opens an outbound HTTPS long-poll to OpenAI and forwards MCP JSON-RPC to a local MCP server over **stdio or HTTP**; streamed results are supported. It serves private / developer-mode connections, not public plugin distribution.
- Apps SDK ([reference](https://developers.openai.com/plugins/reference)): tool-result `_meta` is delivered only to the widget and hidden from the model; `_meta.ui.visibility` can make a tool callable only from the UI; widgets call tools with `window.openai.callTool`; widget CSP `connectDomains` governs fetch/XHR.
- Codex Cloud ([docs](https://developers.openai.com/codex/ide/cloud-tasks)) runs tasks in published environments built from GitHub repositories. The CLI offers `codex cloud exec`, `codex cloud list` and `codex apply` (applies a cloud diff via `git apply`, failing on conflict). A fully scriptable environment/task lifecycle is an open request ([openai/codex#24777](https://github.com/openai/codex/issues/24777)).
- First-party ChatGPT Computer Use exists on the macOS/Windows desktop app (§1.1).
- The intended CodexBridge upstream is `Fanch-hui/codex-bridge`. Phase 0 audited its actual source, LICENSE/NOTICE, module boundaries, unchanged build and smoke at `win` / `v1.3.4`, commit `7844bb608a9a4e96ed09c084589b7825db77aa3e`; the [capability/reuse matrix](evidence/phase0-upstream.md) records failures and limits rather than treating documentation as implementation proof.

### 3.3 Evidence gaps

| Gap | Blocks | Closure |
| --- | --- | --- |
| CodexBridge upstream commit/module boundaries | Concrete upstream reuse | **CLOSED** by the [Phase 0 source/build audit and capability/reuse matrix](evidence/phase0-upstream.md); no upstream source or types imported |
| WebRTC inside the ChatGPT widget sandbox (web, desktop, mobile) and its CSP interaction | Phase 2 | Phase 0 spike |
| TCC attribution of the LaunchAgent daemon and its harness subprocesses; they must **not** inherit CodeBridge.app grants | Phase 1 security | Phase 0 spike. If attribution leaks, the daemon ships under a separate signing identity and/or spawns harnesses with disclaimed responsibility, before any `computer.input` ships |
| `tunnel-client` supervised-runtime lifecycle and restart behavior with a Streamable HTTP MCP target over a Unix-domain socket | Phase 1 ingress | Phase 0 spike |
| Programmatic Codex Cloud handoff | Phase 5 | Re-evaluate at Phase 5 |

No architecture rule in this document depends on unverified CodexBridge internals.

Phase 0B qualification of the disclaimer contingency above: **CONTINGENCY_NOT_PUBLICLY_SUPPORTED** ([public API investigation](evidence/phase0b-responsibility.md)). Launch environment constraints validate the responsible process; they do not disclaim it. `ppid = 1` is not TCC isolation proof. No private API/SPI or entitlement workaround is implemented. F2 stays frozen and **NOT CLOSED**. First test independent, genuinely signed launchd identities; only a reproducible signed isolation failure warrants a minimal architecture amendment. Missing certificates alone do not warrant one. [Current blockers](evidence/phase0b-closeout.md).

## 4. Domain decomposition

The previous "CodeBridge Core" owned eight things: capability routing, project/workspace, session, permission/approval, identity/credential, skills/tools, event bus and extension registry. That is a god-core: it mixed a stateless request pipeline with stateful domains that have their own invariants. V2 splits it.

```text
Bridge kernel (stateless request pipeline)
  ingress adapter -> caller identity -> capability registry
  -> policy decision -> dispatch -> result shaping + audit event
        |
        v
Domain services (own state and invariants)
  Project      Policy          Secret       Runtime              Computer broker
  roots,       rules,          refs ->      sessions, runs,      computer sessions,
  writable,    approvals,      Keychain     provider sessions,   lease mirror,
  sensitive    grants,                      journal, artifacts,  engine link
  paths        caller ceilings              recovery
        |                                        |
        v                                        v
Provider ports
  AgentProvider (OMP / Codex / OpenCode)    ComputerProvider (native engine over Host IPC)
```

| Component | Owns | Must not |
| --- | --- | --- |
| Bridge kernel | request envelope, caller resolution, capability registry, dispatch, audit emission | hold domain state; branch on concrete providers; decide policy itself |
| Project | project ids, authorized roots, writable flags, sensitive-path rules | execute anything |
| Policy | rules, approval requests, grants, caller ceilings | render UI; accept model-originated decisions |
| Secret | secret references and Keychain access | return secrets to remote callers or write them to the journal |
| Runtime | Session, Run, ProviderSession, journal, Artifact metadata, recovery, notification cursors | treat provider conversation content as CodeBridge lifecycle truth |
| Computer broker | ComputerSession records, policy binding, routing to the engine, journaling controller changes | capture pixels or inject input |
| Native host adapter (CodeBridge.app) | Computer engine (capture, input, InputArbiter), approval presentation, notifications, preview media, updater | hold durable domain state; decide policy |

Removed as Core concepts:

- **Event Bus** — the Runtime journal is the only event backbone. In-process fan-out is a journal feature: commit, then notify. Ephemeral signals (preview statistics, typing progress) are not events.
- **Extension Registry** — Go providers are registered at daemon start; out-of-process providers attach over Host IPC with a capability handshake. V2 has no dynamic plugin loading.
- **Skills** — owned by agent harnesses (OMP skills, Codex skills, `SKILL.md`). CodeBridge does not normalize skills until a concrete consumer exists.
- **Identity / Credential** — split into caller identity (Bridge kernel) and secrets (Secret service).

## 5. Process topology

```text
ChatGPT (web / desktop / mobile)              local MCP clients (opt-in)
        |                                              |
OpenAI Secure MCP Tunnel (OpenAI-hosted)               |
        | outbound HTTPS long-poll                     |
tunnel-client (OpenAI binary, supervised by daemon)    |
        | Streamable HTTP over Unix-domain socket      |
        +----------------------------+                 |
                                     |                 | optional local adapter
                                     v                 v
                              Unix-domain MCP ingress
+-------v---------------------------------------------------------------+
| codebridged — Go, per-user LaunchAgent — AUTHORITY                     |
|   Bridge kernel | Project | Policy | Secret | Runtime | Computer broker |
|   AgentProviders -> harness subprocesses (omp / codex / opencode)      |
|   runtime.db (SQLite WAL) in Application Support                      |
+-------^---------------------------------------------------------------+
        | Host IPC v1 (Unix domain socket, versioned JSON-RPC + binary attachments)
+-------+---------------------------------------------------------------+
| CodeBridge.app — Swift, menu-bar login item — NATIVE HOST ADAPTER      |
|   Computer engine: ScreenCaptureKit, CGEvent, Accessibility, InputArbiter |
|   approval presenter, notifications, runtime views, updater           |
|   preview media (WebRTC, Phase 2)                                     |
|   upstream-derived modules behind CodeBridge-owned adapters           |
+-----------------------------------------------------------------------+
```

### 5.1 Why two processes

**Decision:** two CodeBridge processes — `codebridged` (authority) and `CodeBridge.app` (native host adapter).

Reasons, in priority order:

1. **Lifecycle.** Runs must survive UI quit, crash and update. A user can quit an app at any time; a launchd-owned daemon satisfies "UI restart does not end work".
2. **TCC isolation.** Screen Recording, Accessibility and Input Monitoring are separate macOS privacy boundaries associated with app/process responsibility. Agents run arbitrary shell commands, so the process that launches harnesses must not be attributed the app's Computer privileges. Keeping TCC-bound operations inside CodeBridge.app and launching harnesses from an independently managed daemon creates the intended privilege boundary, but Phase 0 must verify the actual TCC attribution on supported macOS versions (§3.3) before `computer.input` ships.
3. **Platform split.** The Go daemon is cross-platform and already carries the Runtime; the computer engine is per-OS by nature (Swift on macOS).

Rejected alternatives:

| Alternative | Rejected because |
| --- | --- |
| Single Go process using cgo for ScreenCaptureKit/CGEvent | puts TCC grants in the process that spawns agent shells (violates 2); ScreenCaptureKit's async API is awkward from Go; a native UI is still required |
| Single Swift process (rewrite the Runtime) | discards working Go runtime and harness code; regresses Windows/Linux |
| Go runtime as a library inside the Swift app | couples crash domains and lifecycles (violates 1 and 2) |
| Swift app supervising Go as a child process (previous V2 text) | quitting the UI ends runs and couples the agent process tree to the TCC-privileged app, making the required privilege separation difficult to prove and maintain (violates 1 and 2) |

Accepted cost: a versioned Host IPC and a second toolchain. Mitigation: the Host IPC surface is small and **is** the ComputerProvider contract plus UI services ([Provider Contracts](provider-contracts.md) §7).

### 5.2 Responsibilities

- **codebridged**: MCP ingress for every CodeBridge tool; Bridge kernel; Project, Policy, Secret, Runtime and Computer broker; AgentProviders and harness subprocesses; `tunnel-client` supervision; durable store; recovery; notification cursors.
- **CodeBridge.app**: computer engine; approval presentation; notifications; runtime views; preview media; updater; registration of the daemon with launchd; upstream-derived desktop modules.
- **Not CodeBridge code**: `tunnel-client` (OpenAI) and harness CLIs (third party).

### 5.3 Lifecycle ownership

- **launchd owns `codebridged`**: CodeBridge.app registers it as a per-user LaunchAgent (`SMAppService`, `KeepAlive`). It is not a system LaunchDaemon: it must run as the user to reach the user's development environment, Keychain and harness homes.
- **`codebridged` supervises `tunnel-client`**, because the tunnel must outlive the UI.
- **CodeBridge.app is a login item.** Quitting it removes Computer and approval-UI capabilities (fail closed) and leaves runs untouched.
- **Development mode**: `codebridged` may run in a foreground terminal with the same store and IPC.

### 5.4 Data authority

| Data | Authority |
| --- | --- |
| projects, sessions, runs, provider-session bindings, computer-session records, events, artifact metadata, policy rules, grants, approvals | `codebridged` store (`runtime.db`), exclusively |
| secrets | Keychain; the store holds references only |
| who may inject input right now | InputArbiter inside CodeBridge.app; volatile; starts with `controller = none` after any restart |
| conversation content of harness sessions | provider-native stores (OMP JSONL, Codex rollouts, OpenCode DB); CodeBridge keeps locators and projections |
| TCC permission state | the OS; queried on use, never cached as truth |

CodeBridge.app keeps UI preferences only.

### 5.5 Failure and restart matrix

| Event | Runs | Computer | Recovery |
| --- | --- | --- | --- |
| `codebridged` crash | harness processes lose their supervisor; on restart non-terminal runs become `interrupted`; orphaned process groups are found by recorded process identity and terminated, or reattached where a provider supports it (Phase 3) | engine drops agent control when IPC drops (`controller = none`) | launchd restarts the daemon; callers resume from journal cursors |
| CodeBridge.app crash or quit | unaffected | every ComputerSession becomes `suspended`; controller `none`; all frames invalid | app restart re-handshakes; the agent must observe again |
| tunnel or `tunnel-client` down | unaffected | in-flight model actions fail; agent lease expires per policy | supervisor restarts `tunnel-client`; callers query state |
| ChatGPT closes | unaffected | lease expiry → controller `none` | — |
| update | updater asks the daemon to prepare; restart is deferred while runs are active unless the user forces it (forced → `interrupted`, resumable in Phase 3) | sessions end | forward-only store migration with automatic backup |
| sleep / screen lock / screen saver / fast user switch | Run behavior follows the OS/provider; Phase 0 records any provider-specific interruption behavior | Computer must move to `suspended`, set `controller = none`, invalidate frames/geometry and discard queued input on the detected transition; no input may replay after return | Phase 0 verifies the actual ScreenCaptureKit/CGEvent behavior; resume requires a fresh observation |

### 5.6 Version skew

- Daemon and app ship in one bundle with one version.
- Host IPC handshake carries protocol `major.minor` and capability flags. A major mismatch disables Computer and UI services (fail closed). Minor versions N and N-1 interoperate to cover update windows.
- The store carries `schema_version`; migrations are forward-only, preceded by a backup; downgrade is unsupported.
- MCP tools change additively inside V2; a breaking change gets a new tool name.

## 6. Ingress and caller identity

Every request becomes a transport-neutral envelope:

```text
Request
  request_id
  caller        {class, principal, ingress, hints}
  capability    tool name
  args
  session_id?   project_id?
  idempotency_key?
```

| Caller class | Origin | Ceiling |
| --- | --- | --- |
| `remote_ai` | Secure MCP Tunnel — ChatGPT model or widget | locally configured remote ceiling; the model never approves; a widget approves only through the opt-in `remote_human` channel with a `_meta`-delivered approval token (§13.2) |
| `local_ui` | CodeBridge.app over Host IPC; approval requires an explicit local user action in the native approval UI | may approve; v1 trusts the signed app + explicit UI gesture, not a biometric proof of presence |
| `local_mcp` | local MCP clients through the stdio shim | opt-in; `computer.*` disabled by default |
| `agent_internal` | an agent run started by CodeBridge calling back with a run-scoped token | at most the parent run's grants |

Rules:

- **No unauthenticated loopback TCP listener.** The default binding is a stdio shim connected to a Unix domain socket in a `0700` per-user directory with a peer-UID check. A loopback HTTP binding, if ever enabled, requires a bearer secret and Host/Origin validation (DNS rebinding).
- **The tunnel is the remote authentication boundary.** Associate it only with the user's own ChatGPT workspace and Platform organization. `openai/subject` may be pinned as defense in depth; it is a hint, not authentication. V1's embedded OAuth authorization server is not part of the personal path.
- **Widget calls are model-indistinguishable at the transport.** They arrive through the same tunnel. They are distinguished only by possession of a grant delivered in tool-result `_meta`, which the model never sees (§12).
- **No TCC confused deputy.** Same-user local processes already have the user's file and shell powers, but not the Screen Recording / Accessibility grants given to CodeBridge.app. Local ingress must not hand those out: `computer.*` is served to `remote_ai` through the daemon-supervised tunnel ingress (authenticated with a per-launch secret) and to `local_ui`; `local_mcp` needs explicit opt-in. This narrows, but does not eliminate, same-user risk; the residual risk is documented.

## 7. Domain model

```text
Project            authorization boundary for files, shell and agents
Session            durable working context; groups work; scope for "session" grants; never completes
 ├─ Run            one execution with a state machine and a terminal result
 ├─ ProviderSession harness-native conversation binding (OMP session, Codex thread, OpenCode session)
 └─ ComputerSession leased control of one computer target
Event              journal entry on a stream
Artifact           metadata for a durable output
```

### 7.1 Entities

Conceptual fields; exact schemas are defined in Phase 0.

**Project** — `prj_…`; name; roots; writable flag; sensitive-path policy.

**Session** — `ses_…`; zero or one `project_id`; title; origin (caller class, external-conversation hint); status `open | archived`. A Session has no running state and no result. It groups Runs and ComputerSessions and is the scope of `session` grants.

**Run** — `run_…` (V1 id format kept); `session_id`; `project_id`; kind `agent | cloud`; executor (provider, model, autonomy mode); optional `role`, `parent_run_id`, `continues_run_id`, `provider_session_id`; status (the V1 state machine unchanged); process identity (pid, start time, process group) while running; final output reference; error category; `last_seq`.

**ProviderSession** — `psn_…`; provider; native id; locator (local only, never exported); `project_id`; capabilities (resume, history). At most one active Run per ProviderSession.

**ComputerSession** — `cmp_…`; `session_id`; optional `run_id` when an agent run drives it; target; state; controller `{kind: agent | human | none, holder, channel: local | remote}`; `controller_epoch`; lease `{holder, expires_at}`; granted verbs and app selectors; `last_frame_id`; `geometry_generation`; capabilities. A ComputerSession is a leased resource, not an objective with a result, so it is **not** a Run.

**Artifact** — `art_…`; producer (run or computer session); kind; reference (path, git ref, provider URI); digest; size; sensitivity; retention class. Created only for durable outputs: patch, commit, branch ref, report, test result, cloud result, or a screenshot the user explicitly saves. Observation frames are not artifacts.

### 7.2 What is deliberately not an entity

- **Turn.** Harnesses define turns differently (Codex turns, OMP `turn_end`, one-shot `exec`), and computer use has action batches instead. A stored Turn would duplicate provider-native state. Turn is an optional `turn_ref` on events plus a UI projection.
- **AgentContext / ComputerContext / CloudContext.** Replaced by typed records: ProviderSession, ComputerSession and Run `kind = cloud`. Generic context bags become dumping grounds.

### 7.3 Naming rule

The bare word **Session** always means the CodeBridge Session. The other two are always qualified: **ProviderSession** and **ComputerSession**. A ChatGPT conversation is an **external conversation**.

### 7.4 Shared contracts, separate state machines

The previous rule "one lifecycle model for agent, computer and cloud" is replaced: domains share IDs, the journal, policy and the artifact contract, but they do **not** share a state machine. A Run (objective → terminal result) and a ComputerSession (leased resource with a controller) are different things.

### 7.5 Relations

- Resuming an interrupted or finished agent run creates a new Run with `continues_run_id` on the same `provider_session_id`.
- An orchestration child is a Run with `parent_run_id`.
- A cloud continuation is a Run `kind = cloud` with `continues_run_id`.
- A Run is never migrated between executors.

### 7.6 Session resolution

An explicit `session_id` must belong to the caller's principal. Otherwise the Bridge uses the external-conversation hint plus project to find an open Session, or creates one. A session id alone never authorizes anything.

### 7.7 V1 compatibility

`agent_runs` maps to Run `kind = agent` plus new columns; `agent_run_events` maps to events on stream `run:<id>` with their existing `seq`. The RunStatus machine and RunManager concurrency, cancellation and timeout semantics are unchanged.

## 8. Event journal

```text
RuntimeEvent
  pos           global monotonic journal position (local subscription cursor)
  stream        run:<id> | computer:<id> | session:<id> | system
  seq           contiguous per stream (gap detection, idempotent replay)
  at
  session_id    project_id?
  source        bridge | runtime | agent | computer | policy | system
  kind
  turn_ref?     correlation_id?
  payload       bounded, metadata first
  raw_ref?      local locator of a provider raw record
```

Sequence decision:

- **per-stream `seq`** generalizes the V1 per-run `seq`; V1 events keep their numbers;
- **global `pos`** serves "everything after X" subscriptions (UI, notifications) with one cursor instead of one per run;
- **no per-session sequence**: Sessions are long-lived and run work concurrently; a per-session counter would serialize unrelated writers without adding a guarantee.

Authority and durability:

- The journal is authoritative for CodeBridge lifecycle facts: run state, approvals and grants, controller changes, artifact creation.
- A state change and its event commit in **one transaction** (V1 writes them separately).
- Terminal run transitions and approval decisions are durable before they are acknowledged.
- Live delivery is a notification after commit; subscribers recover by cursor.
- Provider-native history remains authoritative for conversation content.

Never journaled: pixels, video, secrets, full file contents, hidden model reasoning. Computer events carry frame metadata (id, dimensions, digest, display, epoch), never images.

Retention: lifecycle and policy events are retained; `harness.raw` and verbose provider events are subject to retention limits (V1 retains them forever).

Initial kinds: `run.*`, `agent.message`, `agent.tool.*`, `provider_session.bound`, `policy.approval.requested`, `policy.approval.decided`, `computer.session.*`, `computer.observed`, `computer.action`, `computer.controller.changed`, `artifact.created`, `host.health`.

## 9. Providers

Summary; details in [Provider Contracts](provider-contracts.md).

- **Frozen now** (an implementation exists or is Phase 1): AgentProvider, ComputerProvider (the Host IPC `computer` service), the capability descriptor and the event-emission contract.
- **Internal ports**, not public extension points: ingress adapters, SecretStore, ApprovalPresenter, Notifier, TunnelSupervisor.
- **Deferred, intentionally undefined**: CloudProvider, workspace checkpoints (formerly SnapshotProvider), a standalone MediaProvider, SkillProvider, ControlPlane/Fleet and sandbox execution. Only their binding constraints are written down.

Dependency rules:

- Providers depend only on the port they implement, the event-emission contract and the error model.
- **Providers never call other providers.** Composition happens in domain services (Runtime coordinates checkpoint and cloud) or through Bridge requests (an agent that needs the computer calls CodeBridge tools as `agent_internal`).
- The Bridge kernel never imports a concrete provider.

## 10. Runtime reliability

- A started run is owned by `codebridged`, never by the request that started it.
- ChatGPT, tunnel and UI disconnects do not end runs (§5.5).
- Recovery: on start, non-terminal runs become `interrupted` (V1 behavior, kept). Recorded process identity locates orphaned harness process groups; they are terminated unless the provider supports reattach. Resume creates a new Run on the same ProviderSession.
- **Notification without model polling.** Over Secure MCP Tunnel, the ChatGPT side receives data only in response to a request it initiated. Therefore:
  - the model never polls;
  - a visible widget may hold a bounded long-poll (`events_wait(cursor, timeout)`, a UI-only tool) and post a follow-up message on terminal events;
  - CodeBridge.app posts local notifications;
  - status tools exist for explicit user checks and recovery.
- No Manager, SSE relay or public push service is required.
- The store moves from `~/Library/Caches` to `~/Library/Application Support/CodeBridge/`.

## 11. Computer

### 11.1 Components

- **Computer broker** (daemon): ComputerSession records, policy binding, MCP tools, journaling.
- **Computer engine** (CodeBridge.app): ScreenCaptureKit capture, CGEvent injection, Accessibility queries and the **InputArbiter**. The engine is the only component that posts input events.

### 11.2 Targets

| Target | Status |
| --- | --- |
| `display` | Phase 1 input and observation target |
| `window`, `application` | observation filters and input scope now; primary targets later (window moves and occlusion make window-relative input unsafe without extra checks) |
| `browser` | future separate target/provider with `browser.*` permissions, because DOM access exposes cookies and credentials |
| `virtual` | future isolated desktop or VM |

### 11.3 Frames and coordinates

- Every observation produces a **frame**: `frame_id`, display id, image size, display logical size, scale factor, display origin in global coordinates, capture time, `arbiter_instance`, `controller_epoch`, `geometry_generation`, excluded windows.
- Model coordinates are in the **image space of the referenced frame**. The engine maps image → display logical points → global CoreGraphics coordinates. Retina scale and model-size downscaling are invisible to the model.
- Images are bounded in dimensions and bytes; downscaling is recorded in the frame.

### 11.4 Controller state machine

ComputerSession states: `starting → active ⇄ suspended → closed`, or `failed`.

`controller ∈ {agent, human, none}`; `human` carries a channel, `local` or `remote`; `holder` identifies the ComputerSession and caller that hold control.

- **One input holder per host GUI login session.** Keyboard focus is host-global; multiple displays do not create independent input channels. Observation-only sessions may run concurrently.
- Every controller change increments `controller_epoch`, which is monotonic within one `arbiter_instance`. A restart creates a new instance.
- **Initial acquisition** `none → agent` requires a `computer.input` grant and a lease.
- `agent → human` (takeover) is immediate.
- `agent → none` on pause, lease expiry, local input preemption, emergency stop, permission revocation, target display change or IPC loss.
- `human → none` when the human link is lost — never `human → agent`.
- **Human hold.** Any human-originated transition (takeover, pause, local-input preemption, emergency stop) and any loss of a human link put the session on human hold. Leaving human hold toward `agent` requires an explicit human Resume; the model cannot reclaim control. Other transitions to `none` (lease expiry, display change) allow re-acquisition under a still-valid grant.
- IPC loss, app restart or daemon restart end agent control for every session; work continues only in a newly opened or re-activated session with fresh observation.

### 11.5 Input validity — why `controller_epoch` alone is not enough

An epoch check alone misses actions already being executed, input planned against an outdated screen, geometry changes and checks made in a different process from the injection. Therefore every action batch cites the `frame_id` it was planned from, and the engine executes only if:

1. the ComputerSession is active, holds input and has `controller = agent`;
2. the frame's `arbiter_instance` and `controller_epoch` equal the current values — which also forces a fresh observation after any controller change, including Resume;
3. the frame's `geometry_generation` equals the current one (resolution, arrangement or scale change invalidates it);
4. the frame is younger than the policy bound;
5. the current input target is inside the granted app scope and is not a protected surface (§11.7): pointer events re-hit-test the window/application under the point; keyboard events re-check the focused/frontmost application and focused target.

Checks 1–5 repeat immediately before every low-level event post on a single serialized injector. Pointer hit-testing and keyboard-focus checks therefore happen at injection time, not only once per batch. A change mid-batch aborts the remainder. On abort or preemption the engine releases every key and button it holds. Batches run in order and stop at the first failure.

macOS can still change focus/window ordering between the final OS query and CGEvent delivery. CodeBridge cannot prove delivery to the originally intended window against an adversarial or concurrent window-manager change; this is a documented residual race. Any observable focus, scope or protected-surface change causes immediate abort.

Validation and injection run in the same process. Validating in the daemon and injecting in the app would leave a cross-process time-of-check/time-of-use window.

### 11.6 Human/agent exclusivity

Invariant: **CodeBridge never injects agent input while a human controls the machine or is physically using it.**

- **Local input preemption.** A listen-only event tap detects hardware input not originated by the engine. While `controller = agent`, it preempts to `none` (epoch++) before the next agent event. If the monitor cannot be installed (permission missing), `computer.input` is unavailable — fail closed.
- **Emergency stop.** Menu-bar control and a global shortcut, handled in-process by the arbiter.
- **Remote human input** (Phase 2) enters the engine through the preview data channel only while `controller = human(remote)`.

### 11.7 Protected surfaces

Capture excludes CodeBridge's own windows (approval prompts, grants). Input is denied to CodeBridge's own windows, to OS security surfaces (System Settings privacy panes, authentication and security agents, Keychain Access) and to user-configured apps such as password managers. Protected-surface and app-scope checks are re-evaluated immediately before every injected pointer or key event. An agent must never be able to approve its own permission by clicking.

### 11.8 Permissions and privacy

- `computer.observe` — screenshots leave the machine to the AI provider; the user is told so.
- `computer.input` — always carries app selectors.
- `computer.preview` — Phase 2.

Grants are bounded by the ComputerSession lease. TCC permissions are checked at session start and on every use, because they can be revoked at any time. Frames live in memory only and are not persisted by default.

### 11.9 Multi-display, resolution and app switching

- Phase 1 binds a session to one display; input outside it is rejected.
- Display reconfiguration increments `geometry_generation`; if the target display disappears the session is suspended.
- Switching apps or windows is allowed inside the app scope; input that would land outside it is rejected.
- Sleep or screen lock suspends the session.

### 11.10 Action model

**Primitives (frozen):** `move`, `click`, `double_click`, `drag`, `scroll`, `type`, `keypress`, `wait`. The wire format is a tagged union with capability negotiation; unknown kinds are rejected as `unsupported`. `type` and `keypress` require a fresh focused/frontmost-app scope check immediately before each emitted key event; a focus change to an out-of-scope or protected target aborts the remainder.

Not computer actions:

- `script` (AppleScript, shell-driven UI automation) is a shell/automation capability under `shell.execute`-class policy, because it bypasses frame and app-scope checks.
- `browser_script` belongs to a future browser target/provider with `browser.*` permissions.
- `workflow` is a Runtime concern (a Run or orchestration) composed of primitives.

A future observation mode may return an accessibility tree (`observe.ax_tree` capability).

## 12. Data planes

- **Model plane (MCP):** observations as MCP image content plus frame metadata; action batches; status.
- **Human plane (WebRTC, Phase 2):** live video from the same capture pipeline to the ChatGPT widget; remote human input over a data channel.

Rules:

- live video never enters model context;
- model control never depends on WebRTC;
- media failure never changes ComputerSession state, except that losing a remote human controller sets `controller = none`;
- preview is never recorded by default.

Minimal infrastructure — **no Manager**:

- **Signaling:** UI-only MCP tools (`_meta.ui.visibility = ["app"]`) through the same tunnel; SDP with gathered candidates. No public signaling service by default.
- **Grants:** the daemon mints short-lived view/control grants bound to one ComputerSession, delivered in tool-result `_meta` (widget only, hidden from the model), bound to the widget instance (`openai/widgetSessionId`) when available, revoked on stop.
- **STUN:** public or configured.
- **TURN:** the only required public component, for networks where direct ICE fails. A stateless relay; credentials are minted locally per viewer with time-limited HMAC (TURN REST style). It sees only DTLS-SRTP ciphertext.
- **Fallback:** if widget WebRTC or tool-based signaling proves infeasible (Phase 0 spike), use a stateless rendezvous relay — a mailbox keyed by an unguessable grant id, with no accounts and no session state. It must not grow into a Manager.

## 13. Security model

### 13.1 Permissions

Permission = **verb + resource selector**.

- Verbs: `filesystem.read`, `filesystem.write`, `shell.execute`, `agent.start`, `agent.cancel`, `computer.observe`, `computer.input`, `computer.preview`, `git.push`, `cloud.upload`, `cloud.execute`, `network.connect`, `secret.read`.
- Selectors: project and path glob; command prefix; app bundle id; host and port; secret id; provider and autonomy mode.
- Effects: `allow`, `deny`, `ask`. Deny always wins. Unmatched means `ask`.
- Grant scopes: `once`, `session`, `project`, `always`. Computer grants are also bounded by the lease; `project` scope does not apply to computer verbs.

### 13.2 Approval channels

| Channel | Allowed scopes | Notes |
| --- | --- | --- |
| `local_ui` | all | strongest; every decision requires an explicit local user action in CodeBridge.app |
| `remote_human` (widget, opt-in) | `once`, `session` | available from Phase 1 for approval-only flows; never `project` / `always`; never `secret.read` or `cloud.upload`; requires an approval token delivered only in `_meta` |
| model | none | no model-visible tool can create or widen a grant; V1 `permission_grant` is not carried forward |

**Local-human trust boundary:** v1 treats a code-signature-verified CodeBridge.app plus an explicit foreground approval gesture as `local_ui`. It does not require LAContext / Touch ID for every approval. A compromised app process remains a documented residual risk; stronger authenticated-user-presence requirements may be added for selected high-impact grants without changing the Policy model.

**Attenuation:** effective permission = local policy ∩ caller-class ceiling ∩ parent-run grants. Remote callers cannot broaden local permissions; child runs cannot exceed their parents.

### 13.3 Harness autonomy

V1 harnesses run with their own auto-approval flags, so `agent.start` already authorizes the harness's full autonomy. V2 makes this explicit: providers declare autonomy modes (for example read-only, workspace-write, full host), and Policy grants `agent.start` per autonomy ceiling. Routing harness-native approval requests into Policy is a later provider capability.

### 13.4 Enforcement limits

Stated, not hidden:

- command-prefix rules are a UX filter, not a sandbox;
- `network.connect` is enforced only for CodeBridge-initiated egress (uploads, TURN, cloud APIs), not for agent subprocess traffic on the native host;
- real isolation requires optional sandbox execution (Phase 7).

### 13.5 Secrets, retention and IPC

- Secrets live in Keychain; the store keeps references. Secrets never enter the journal, MCP results or model context, and are injected only into the provider that owns them.
- Frames are memory-only; preview is never recorded; raw harness events are retention-limited; every artifact has a retention class.
- Host IPC uses a socket in a `0700` per-user directory, checks peer UID, verifies the code signature of a peer claiming the app role, and negotiates versions.
- Project roots under macOS TCC-protected locations are never assumed accessible. Phase 0 must establish whether `codebridged` can hold a stable per-user Files-and-Folders authorization across rebuilds/updates. Project registration probes the root: if the required host permission is missing, it fails with `permission_denied` reason `host_permission_required`; if stable daemon authorization is not viable, protected roots are explicitly unsupported in v1 rather than failing later as opaque I/O errors.

### 13.6 Computer fails closed

Input stops on any of: missing or revoked TCC permission; IPC loss; daemon restart; lease expiry; unknown arbiter state; failed local-input monitor; display change; protected-surface hit.

### 13.7 Upstream code

Upstream-derived code runs inside CodeBridge.app with its TCC grants. Every upstream sync is a security review.

## 14. Orchestration

**CodeBridge owns:** run-tree bookkeeping (`parent_run_id`, `role`), permission attenuation and budget enforcement across the tree, provider/model routing (ModelPolicy), cross-provider correlation, durable state.

**Providers own:** their internal multi-agent protocols (OMP subagents, Codex subagents, OpenAI Agents SDK handoffs), prompting and planning.

**Minimal abstraction:** an orchestrator is any AgentProvider run that starts child runs through CodeBridge tools as `agent_internal`; CodeBridge links them automatically. ModelPolicy maps role → (provider, model, autonomy). BudgetPolicy defines wall-clock, token and cost ceilings that children inherit.

**Deferred:** CodeBridge-native TaskGraph and ReviewLoop engines. Revisit only if OMP-first orchestration through tools proves insufficient.

## 15. Local ↔ Cloud handoff

- A handoff creates a **new Run** (`kind = cloud`, `continues_run_id`). Runs are never migrated.
- **Single writer:** exactly one active run per continuation chain.
- **Local authority:** the local workspace is authoritative for local files. Cloud results return as artifacts (patch or branch). Applying them is an explicit local operation into a new branch or worktree with conflict detection — never silently onto a dirty tree.
- Chain metadata stays in the local journal; cloud state is mirrored as events.
- **Checkpoints** are git-based: base commit plus a WIP commit on a private ref (`refs/codebridge/checkpoints/…`). They are prepared incrementally at run milestones and idle points while the machine is awake, because a sleep notification leaves too little time to upload. Untracked files are included only through an allowlist; sensitive paths are excluded (V1 sensitive-path rules are reused).
- Upload is separately permissioned (`git.push` / `cloud.upload` with an explicit destination).
- **Codex Cloud reality:** environments are configured server-side from GitHub repositories; tasks are submitted with `codex cloud exec`; diffs come back through `codex apply`. CodeBridge must not assume SSH, VM semantics, arbitrary snapshot upload or an uploaded environment manifest. A Codex Cloud handoff therefore means: push the checkpoint branch → submit a task that references it → fetch the diff as an artifact.
- CloudProvider and checkpoint contracts stay undefined until one real provider is integrated (Phase 5).

## 16. Upstream strategy

CodexBridge is an implementation source, not an owner. Phase 0 pinned `7844bb608a9a4e96ed09c084589b7825db77aa3e` (`win`, `v1.3.4`) and recorded per-module reuse decisions ([audit](evidence/phase0-upstream.md)); no upstream source was imported. Reuse modes, the adapter layer, the patch queue and the sync procedure are defined in [Migration](migration.md) §4. Binding rules:

- CodeBridge owns the MCP surface, Policy, Runtime and Computer contracts; upstream code never registers or routes CodeBridge tools.
- Upstream code is wrapped by CodeBridge-owned adapters; upstream types never cross into CodeBridge domain code or Host IPC, and CodeBridge types never enter upstream code.
- CodeBridge.app has its own bundle id and signing identity; TCC grants are never shared with an upstream app.

## 17. Fleet

- **No ControlPlaneProvider.** "LocalControlPlane" was simply the daemon, and no second implementation exists.
- **The Manager is not part of V2.** V1 Manager/Client stays buildable at the frozen checkpoint.
- A future fleet attaches as an optional outbound connector from `codebridged`: inventory, health and version metadata out; managed policy in. Managed policy can only restrict (deny-wins layering). Fleet is never in the request data path.
- V1 code worth revisiting then: device enrollment and rotation (`authstore`), the JWT verifier (`oauthresource`), the audit format, the `RunEventBroker` replay logic and registry backpressure.

## 18. Storage

- Location: `~/Library/Application Support/CodeBridge/` (`runtime.db`, `artifacts/`); logs in `~/Library/Logs/CodeBridge/`.
- May persist: project, session, run and provider-session metadata; ordered events; provider locators; artifact metadata and explicitly retained payloads; terminal results; policy rules and grants; checkpoint metadata; UI indexes.
- Never persisted by default: screen video, screenshots, secrets, source copies for telemetry, hidden model reasoning.

## 19. Decision register

### 19.1 Frozen

| ID | Decision |
| --- | --- |
| F1 | `codebridged` is the single authority; CodeBridge.app is a host adapter without durable domain state |
| F2 | Two processes; launchd owns the daemon; only the app holds TCC grants; harnesses never inherit them |
| F3 | MCP ingress terminates in the daemon; `tunnel-client` is transport only; no unauthenticated loopback TCP |
| F4 | Caller classes, approval channels and attenuation; no model-visible path creates or widens grants |
| F5 | Entities: Project, Session, Run, ProviderSession, ComputerSession, Event, Artifact; Turn is not an entity |
| F6 | Journal: per-stream contiguous `seq` + global `pos`; state change and event in one transaction |
| F7 | Computer: frame-cited actions; arbiter and injector in one process; one input holder per host; local-input preemption; human-only resume; protected surfaces; fail closed |
| F8 | Primitive action set; `script` / `browser_script` / `workflow` are not computer actions |
| F9 | MCP/WebRTC separation; no Manager; TURN is the only required public component |
| F10 | AgentProvider, ComputerProvider, capability descriptor, event-emission and error contracts (v1) |
| F11 | Handoff = new Run + continuation relation; local authority; explicit apply |
| F12 | No ControlPlane interface; a future fleet can only restrict |
| F13 | Upstream behind CodeBridge-owned adapters; upstream never imports CodeBridge, CodeBridge domain never imports upstream types |

### 19.2 Deferred

| ID | Decision | Trigger |
| --- | --- | --- |
| D1 | CloudProvider and checkpoint contracts | first real cloud integration (Phase 5) |
| D2 | Standalone MediaProvider | a second media source besides computer preview |
| D3 | SkillProvider | a concrete consumer needs cross-harness skill normalization |
| D4 | CodeBridge-native TaskGraph / ReviewLoop | OMP-first orchestration through tools proves insufficient |
| D5 | window / application / browser / virtual as primary input targets | display MVP proven |
| D6 | Separate Swift computer helper process | UI crashes measurably hurt computer reliability |
| D7 | Routing harness-native approvals into Policy | per provider, Phase 3+ |
| D8 | OAuth through the tunnel for multi-user access | a tunnel must serve more than one person |
| D9 | Sandbox execution | Phase 7 |
| D10 | Orphan reattach vs. terminate | per provider, Phase 3 |
| D11 | Reuse mode per upstream module | Resolved by the [Phase 0 audit](evidence/phase0-upstream.md); actual imports still require the adapter/attribution procedure |

## 20. Architectural invariants

1. The Bridge kernel and domain services never depend on a concrete agent, OS, transport, media or cloud provider.
2. Every capability use enters through the Bridge kernel and Policy; providers never call providers.
3. The `codebridged` store is the single durable authority for CodeBridge state.
4. Domains share IDs, journal, policy and artifact contracts; they keep separate state machines where semantics differ.
5. The native host is the default execution environment.
6. No model-visible path can create or widen a permission.
7. Agent input is never injected while a human controls or touches the machine; Computer fails closed.
8. Model observation and human media remain separate planes.
9. Agent subprocesses never hold the app's TCC grants.
10. CodexBridge is an implementation source behind adapters, never the owner of CodeBridge's domain or MCP surface.
11. Fleet, cloud and media infrastructure stay optional; only a TURN relay may sit in the default personal path, and only when direct media fails.
