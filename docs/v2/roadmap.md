# CodeBridge V2 Roadmap

Status: **implementation roadmap — PHASE_0_BLOCKED; Phase 1 NOT READY** ([closeout](evidence/phase0-closeout.md)); frozen architecture decisions unchanged.

The architecture is Bridge-first; the roadmap is Computer-first. **Computer Use is the first major V2 feature.**

Each phase builds the long-term skeleton it needs and nothing more. Phase 1 builds the daemon, policy and journal foundation that Computer Use itself requires, so Computer Use never ships through a temporary architecture.

## Phase 0 — Baseline, decisions and spikes

Goal: close the evidence gaps that the architecture depends on, before feature code.

Documents:

- [x] V2 architecture baseline.
- [x] Provider contracts.
- [x] Migration strategy.
- [x] Architecture review and revision (2026-10-02).

Upstream:

- [x] Identify CodexBridge upstream: `Fanch-hui/codex-bridge`, public `win` development branch, Apache-2.0.
- [x] Pin the exact upstream commit and audit language/stack/module boundaries: `win` / `v1.3.4`, `7844bb608a9a4e96ed09c084589b7825db77aa3e` ([audit](evidence/phase0-upstream.md)).
- [x] Record Apache-2.0 LICENSE/NOTICE obligations for each proposed reuse; no upstream module imported in Phase 0.
- [x] Build upstream unchanged: SwiftPM core + native service smoke PASS; `.app` Xcode build stalled and is **not verified**; documented tests report no test targets.
- [x] Fill the capability matrix ([Migration](migration.md) §4.2) and choose a reuse mode per module ([complete observed matrix](evidence/phase0-upstream.md)).
- [x] Upstream decision recorded: no source imported in Phase 0; per-module package/copy/vendor/no-reuse choices recorded; no Phase 1 item depends on upstream.

Spikes:

- [ ] Ingress: `tunnel-client` → Streamable HTTP MCP over Unix-domain socket → `codebridged` → a `ping` tool answered in a real ChatGPT developer-mode conversation.
- [ ] Lifecycle: `SMAppService` LaunchAgent with `KeepAlive`; quitting the app leaves the daemon running; daemon crash restarts.
- [ ] TCC attribution: confirm the daemon and its harness subprocesses are **not** attributed to CodeBridge.app's Screen Recording / Accessibility grants.
- [ ] Daemon protected-root TCC: test a Project under `~/Documents` (and equivalent protected roots), establish whether the per-user LaunchAgent can hold a stable Files-and-Folders authorization across rebuild/update, and choose one documented v1 outcome: supported with `host_permission_required` guidance, or explicitly unsupported.
- [x] Native host: actual launchd daemon ran shell, git, Docker Desktop **Server**, SSH and kubectl ([parent smoke](evidence/phase0-parent-smoke.json)); cwd `/`, captured native PATH/HOME, no Manager or container execution.
- [ ] Lock-state behavior: measure ScreenCaptureKit/CGEvent behavior for screen lock, screen saver, wake and fast user switching; verify queued input cannot replay after unlock and feed the observed behavior into the §5.5 failure matrix.
- [ ] Widget approval feasibility (gates Phase 1 remote approval only): UI-only approval tool round trip plus an approval token delivered through tool-result `_meta` on the supported ChatGPT surfaces; no WebRTC required.
- [ ] Widget media feasibility (gates Phase 2 only): `RTCPeerConnection` inside the ChatGPT widget on web, desktop and mobile; signaling round trip through a UI-only tool.

Definitions:

- [ ] App identity: stable bundle/signing identifiers and a real Team ID; Apple Development preferred for development, Developer ID Application also accepted (TCC grants must survive rebuilds).
- [x] Host IPC v1 schema: `host`, `computer`, `approval`, `notify`; real Go/Swift UDS handshake and negative security/version paths passed ([IPC](evidence/phase0-ipc.md)); only the Phase 0 hello/health surface is implemented.
- [x] Store schema v1: projects, sessions, runs, provider_sessions, computer_sessions, events (`stream`, `seq`, `pos`), artifacts, policy rules, grants, approvals, `schema_version` ([transaction/reopen smoke evidence](evidence/phase0-store.md)).
- [x] Capability descriptors for the Phase 1 tools ([schema smoke evidence](evidence/phase0-capabilities.md)); definitions only, Computer tools remain unavailable in Phase 0.

Phase 0 outcome: upstream audit, native host, IPC, Store and capability definitions passed. Manual LaunchAgent restart and local MCP/widget smoke do **not** satisfy signed SMAppService or actual ChatGPT acceptance. F2 is an architecture blocker: a real daemon-owned harness captured with daemon-attributed TCC authorization; original App-grant causality is unknown ([TCC](evidence/phase0-tcc.md) §7). Signed protected-root retention, real lock/saver/sleep/FUS transitions and reliable physical preemption are unresolved; input stays unavailable. Widget media is **DEFERRED only because it gates Phase 2**, not Phase 1. See [complete task status and prerequisites](evidence/phase0-closeout.md).

Phase 0B: **PHASE0_BLOCKED**, F2 **NOT CLOSED**, Phase 1 **NOT READY**. Current host has zero valid signing identities (**SIGNED_TCC_BLOCKED_EXTERNAL**); actual ChatGPT/Tunnel acceptance is **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**. No signed experiment failed in this pass. The responsibility-disclaimer contingency has no established public API and is not implemented. [Phase 0B evidence and next acceptance steps](evidence/phase0b-closeout.md) retain the earlier daemon capture finding without treating it as proof of signed App-grant inheritance.

Exit:

- ChatGPT → Secure MCP Tunnel → `codebridged` runs a real host command natively, without Manager or Docker.
- The daemon survives app quit; the Host IPC handshake works.
- The upstream decision is recorded, even if it is "not used".

## Phase 1 — Computer Use MVP (**highest feature priority**)

Goal: ChatGPT can safely observe and control the real Mac.

### 1A. Daemon foundation (only what Computer Use needs)

- [ ] `codebridged` skeleton with the Bridge kernel: ingress, caller classes, capability registry, dispatch, audit events.
- [ ] Runtime store and journal: V2 schema in Application Support; transactional state + event; per-stream `seq` and global `pos` (lifting V1 journal code).
- [ ] Policy v2 minimum: verbs + selectors, allow/deny/ask, `once` / `session` / `always` scopes, approvals presented by CodeBridge.app plus an opt-in approval-only `remote_human` widget path (`once` / `session` only). No model-callable grant tool.
- [ ] Project registration probes TCC-protected roots and surfaces `permission_denied: host_permission_required` instead of opaque I/O failure.
- [ ] Session resolution.
- [ ] `tunnel-client` supervision.

### 1B. Native computer engine (CodeBridge.app)

- [ ] Menu-bar app skeleton and Host IPC client.
- [ ] Permission detection: Screen Recording, Accessibility, Input Monitoring; re-checked on use.
- [ ] Display enumeration and geometry; `geometry_generation` driven by display reconfiguration, sleep and lock.
- [ ] ScreenCaptureKit still capture with CodeBridge windows excluded; dimension and byte bounds.
- [ ] Frame stamping: `frame_id`, `arbiter_instance`, `controller_epoch`, `geometry_generation`; image-space → logical → global coordinate mapping (Retina).
- [ ] InputArbiter: controller state machine, epoch, lease, one input holder per host.
- [ ] Single serialized CGEvent injector with per-event revalidation and held-input release.
- [ ] Local physical-input preemption (listen-only event tap); `computer.input` unavailable if the monitor cannot run.
- [ ] Emergency stop: menu bar and global shortcut.
- [ ] App-scope enforcement with per-event pointer hit-test and per-key focused/frontmost-app checks; protected surfaces (CodeBridge windows, OS security surfaces, configured apps) are re-checked immediately before injection.

### 1C. Tools and end-to-end

- [ ] `computer_start`, `computer_observe`, `computer_action`, `computer_status`, `computer_stop`.
- [ ] Observations as MCP image content plus frame metadata.
- [ ] Action primitives: move, click, double_click, drag, scroll, type, keypress, wait; ordered batches stop on first failure; every batch cites `frame_id`.
- [ ] Metadata-only computer events in the journal.

Required E2E (real ChatGPT through the tunnel):

- [ ] observe → act → observe on the real Mac.
- [ ] permission denied and approval-required paths; approval can come only from explicit local UI action or the opt-in `remote_human` widget path; the model cannot approve.
- [ ] remote-started session can obtain a `once` / `session` approval through the approval-only widget without WebRTC.
- [ ] stale frame rejected after a controller change.
- [ ] resolution/arrangement change invalidates frames.
- [ ] physical mouse/keyboard use preempts the agent immediately.
- [ ] pointer and keyboard input into a protected surface or out-of-scope/focused app is rejected.
- [ ] screen lock / screen saver / fast-user-switch behavior matches the Phase 0 decision and cannot replay queued input after return.
- [ ] app quit and daemon restart leave `controller = none`.

Exit:

> A real ChatGPT conversation can observe, click, type, scroll and drag on the user's Mac through Secure MCP Tunnel — without Manager, Docker Client or public MCP infrastructure — and every negative path above fails closed.

Phase 1 exit review: re-check positioning against first-party ChatGPT Computer Use ([Architecture](architecture.md) §1.1).

## Phase 2 — Live preview and remote takeover

Goal: the user watches from the ChatGPT widget and safely takes control.

Prerequisite: Phase 0 widget-media spike passed; otherwise choose the stateless rendezvous fallback before starting.

- [ ] Preview capability in the computer engine (same capture pipeline).
- [ ] View/control grants minted by the daemon, delivered in tool-result `_meta`, bound to the ComputerSession and widget instance.
- [ ] UI-only signaling tools through the tunnel.
- [ ] STUN; TURN with locally minted time-limited credentials.
- [ ] Adaptive resolution / FPS / bitrate; pause when the widget is hidden.
- [ ] ChatGPT Computer widget: Pause, Take over, Resume agent, Stop.
- [ ] Remote human input through the data channel into the arbiter (`controller = human(remote)`).
- [ ] Reuse the Phase 1 `remote_human` approval channel; adding live preview/control must not widen its scopes or trust level.
- [ ] Loss of the human link → `controller = none`, never back to agent.
- [ ] Resume requires a human action; old frames are invalid by epoch.

Exit:

> Human and agent never inject input concurrently, preview failure never affects model control, and no Manager or stateful public service is involved.

## Phase 3 — Runtime and agents on V2

Goal: long agent work runs under `codebridged` with the same Session, Policy and Journal as Computer Use.

Lift from V1:

- [ ] RunManager state machine, concurrency, cancellation, timeouts.
- [ ] Subscriptions and cursor replay over the V2 journal.
- [ ] OMP, Codex and OpenCode as AgentProviders (launch code + session adapters merged).
- [ ] Sensitive-path, patch and checkpoint file tools behind Project.

Add:

- [ ] ProviderSession binding from live output; resume as a new Run on the same ProviderSession.
- [ ] Process identity and orphan handling; per-provider reattach decision (D10).
- [ ] Autonomy modes per provider; `agent.start` granted per autonomy ceiling.
- [ ] `agent_start`, `agent_status`, `agent_result`, `agent_cancel`, `runs_list`; UI-only `events_wait`.
- [ ] Completion delivery: widget long-poll + follow-up message; local notifications.
- [ ] Runtime views in CodeBridge.app: Conversation, Activity, Artifacts, Computer, Raw.
- [ ] Raw-event retention limits.
- [ ] One-time read-only import of V1 `runs.db`.
- [ ] Evaluate Codex `app-server` for approval routing (D7).

Exit:

> Long work continues across ChatGPT, tunnel, UI and daemon restarts; state is recoverable from the local journal; the model never polls.

## Phase 4 — Orchestration (minimal)

Goal: cross-provider work without CodeBridge re-implementing multi-agent protocols.

- [ ] Run tree: `parent_run_id`, `role`; children created by `agent_internal` callers are linked automatically.
- [ ] Permission attenuation and budget inheritance across the tree.
- [ ] ModelPolicy: role → provider / model / autonomy.
- [ ] BudgetPolicy: wall-clock, token and cost ceilings.
- [ ] Cross-provider correlation in the journal.
- [ ] OMP-first orchestrator working through CodeBridge tools.
- [ ] Decide on CodeBridge-native TaskGraph / ReviewLoop only with evidence (D4).

Exit:

> A project routes planner / coder / reviewer / operator roles to different providers and models without Bridge kernel changes.

## Phase 5 — Local ↔ Cloud handoff

Goal: work continues while the laptop sleeps, without making CodeBridge a cloud platform.

- [ ] Git-based checkpoints on private refs, prepared incrementally while awake.
- [ ] Untracked-file allowlist; sensitive paths excluded.
- [ ] Define CloudProvider from the first real integration (D1).
- [ ] Codex Cloud path: push checkpoint branch → `codex cloud exec` → status → `codex apply` diff as artifact.
- [ ] Continuation chain: new Run `kind = cloud`, `continues_run_id`, single writer.
- [ ] Explicit local apply into a new branch/worktree; conflicts surfaced, never silently resolved.

Exit:

> A run hands off to one real cloud provider and returns a diff that the user applies locally without overwriting local changes.

## Phase 6 — Additional platforms

- [ ] `codebridged` on Windows and Linux.
- [ ] Windows ComputerProvider.
- [ ] Linux X11/Wayland ComputerProvider.
- [ ] Platform permission models mapped to the same verbs.

## Phase 7 — Optional sandbox execution

- [ ] Sandbox as an autonomy/environment capability of agent and shell tools.
- [ ] Docker / VM isolation with explicit capability differences from the native host.
- [ ] Never the default personal developer environment.

## Phase 8 — Fleet (only with demonstrated demand)

- [ ] Optional outbound fleet connector.
- [ ] Inventory, health, version metadata.
- [ ] Managed policy layer that can only restrict.
- [ ] Never in the request data path.

## Work explicitly stopped on the mainline

- public Manager MCP ingress;
- mandatory VPS;
- Caddy/Nginx MCP routing;
- self-built OAuth ingress for the personal path;
- Docker Client as the default Mac execution environment;
- Manager→Client gRPC routing as the personal execution path;
- model-callable permission grants;
- duplicating mature desktop/updater/credential features that a pinned upstream already provides;
- adding agent adapters only to increase adapter count.

## Priority rule

When priorities conflict:

1. correctness and security;
2. long-term maintainability of the boundaries in [Architecture](architecture.md) §19;
3. upstream syncability;
4. Computer Use;
5. Runtime durability;
6. orchestration;
7. cloud handoff;
8. additional platforms;
9. fleet.
