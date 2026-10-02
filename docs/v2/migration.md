# CodeBridge V2 Migration Strategy

Status: **migration baseline — revised by the 2026-10-02 architecture review**

V1 checkpoint before V2: `d251a3c Build durable runtime and harness session foundation`.

V2 neither rewrites everything nor continues V1 unchanged. It keeps the Runtime work that differentiates CodeBridge, retires the Manager relay from the personal path, and reuses upstream desktop code only behind CodeBridge-owned adapters.

## 1. Principles

```text
Do not rebuild what a pinned, compatible upstream already solves well.
Do not throw away the durable Runtime that already works.
Do not let upstream internals become CodeBridge's domain model or MCP surface.
Do not carry V1 security shortcuts into V2.
```

## 2. V1 code disposition

Facts from the 2026-10-02 code review ([Architecture](architecture.md) §3.1).

### 2.1 Keep and lift into `codebridged`

| V1 code | V2 destination | Required change |
| --- | --- | --- |
| `internal/agentops/run_manager.go` — RunManager, state machine, concurrency, cancel, timeout, recovery to `interrupted` | Runtime | add session / project / provider-session / process-identity fields |
| `internal/agentops/run_store.go` | Runtime store | V2 schema; Application Support location; state change + event in one transaction; `schema_version` |
| `internal/agentops/run_events.go` — journal, `SubscribeEvents`, `EventsAfter` | Runtime journal | generalize to `stream` / `seq` + global `pos`; retention for raw events |
| `internal/agentops/subagent.go` — CLI harness launch | AgentProvider execution | own process group; process identity; autonomy-mode mapping |
| `internal/agentops/harness_session.go`, `omp_session.go`, `codex_session.go`, `opencode_session.go` | AgentProvider history + live normalization | wire them; today they are tested but unused |
| `internal/agentops/ops.go` file/git/LSP tools, `apply_patch` / `rollback_patch`, security and sensitive-path checks | Bridge tools behind Project | re-register through capability descriptors |
| `internal/manager/tool_output.go` output schemas | capability descriptors | reuse schemas and the validation pattern |

### 2.2 Rewrite

| V1 code | Why |
| --- | --- |
| `internal/agentops/permissions.go` | keep the deny > allow > ask matching idea; rewrite into Policy with verbs, selectors, scopes, approval channels and attenuation. V1 grants are process-global and can be issued by the model |
| `permission_grant` MCP tool | removed: it is model-callable, declared read-only and can persist `always` rules. V2 approvals come only from `local_ui` or opt-in `remote_human` |
| Client local config UI (`internal/ui`) | superseded by CodeBridge.app |

### 2.3 Retire with V1 (keep buildable, do not extend)

| V1 code | Reason | Possible later reuse |
| --- | --- | --- |
| `cmd/client/runtime_events.go` (`run_event`, `run_heads`, `run_replay`) | the journal and the MCP server share one process; reconnect is a cursor read | — |
| `internal/manager` (`Registry`, `grpc_agent.go`, `RunEventBroker`, admin API), `internal/pb`, `internal/protocol` envelopes | Manager relay is not in the V2 path | fleet: registry backpressure, broker replay logic |
| `internal/authserver`, `internal/oauthresource`, `internal/authstore`, `internal/clientcred` | tunnel is the remote boundary; no device enrollment in the personal path | fleet: enrollment, rotation, JWT verification |
| `internal/mcpcallstore` | replaced by journal/audit events | — |
| `internal/auditlog` | metadata-only principle kept; audit events go to the journal | format reference |
| `Dockerfile.client`, `deploy/compose.client.yml`, `deploy/client-entrypoint.sh` | Docker is not the default execution environment | Phase 7 sandbox reference |

V1 already supports a native client (`deploy/launchd`, `deploy/systemd`); V2 drops Docker as the default, not native execution.

## 3. Stop extending on the main path

Freeze rather than delete:

- public Manager MCP ingress and Manager as a mandatory relay;
- Manager→Client gRPC personal execution path;
- Docker Client as the Mac default;
- Caddy/Nginx public MCP routing;
- custom OAuth ingress and device enrollment/account routing built for the relay.

Keep the code until V2 proves the replacement path (§10), then archive or remove it in a dedicated cleanup milestone.

## 4. CodexBridge reuse

### 4.1 Status: upstream identified, not yet pinned

The intended upstream has been externally identified as `Fanch-hui/codex-bridge`, whose public `win` branch documents the expected native desktop/service architecture, Secure MCP Tunnel integration, tasks/sessions, approvals, agent discovery/connectivity and Apache-2.0 licensing. This repository still contains no vendored copy or pinned upstream commit, and CodeBridge has not yet built the upstream unchanged. Phase 0 therefore verifies concrete module boundaries before any reuse decision.

Secure MCP Tunnel itself is OpenAI's `tunnel-client`. CodeBridge supervises it from `codebridged` and targets the daemon's Streamable HTTP MCP endpoint over a Unix-domain socket, so the critical path does not depend on CodexBridge and does not require an extra stdio shim.

### 4.2 Capability matrix (filled in Phase 0)

| Claimed upstream capability | Needed by | Preferred reuse | Fallback |
| --- | --- | --- | --- |
| native desktop shell | CodeBridge.app | package dependency, or copy with attribution | own SwiftUI menu-bar app |
| Secure MCP Tunnel setup UX | onboarding | UI only; the process is supervised by `codebridged` | own setup screen |
| packaging / signing / notarization / updater | release | build scripts and updater framework | own pipeline |
| credential / Keychain | SecretStore | module | own Keychain wrapper |
| approval UI | ApprovalPresenter | UI components only; decisions stay in Policy | own UI |
| project registration | Project | UI only; the registry is CodeBridge's | own UI |
| agent discovery | AgentProvider `Describe` | detection logic | own detection |

### 4.3 Reuse modes, in order of preference

1. **Package dependency at a pinned version, unmodified.**
2. **Copy with attribution** — a one-time port of a small, stable piece into a third-party directory with origin commit and NOTICE; no sync expectation.
3. **Vendored subtree with a patch queue** — only for larger modules that need fixes. Every patch is small, justified and upstreamable.
4. **Running or forking the upstream app and plugging CodeBridge into it** — rejected. That makes CodeBridge an upstream plugin and puts CodeBridge's MCP surface under upstream control.

The identified upstream is Apache-2.0, but Phase 0 must still record LICENSE/NOTICE obligations and the exact origin commit for every reused module before choosing a reuse mode.

### 4.4 Adapter layer

```text
CodeBridge domain (Swift app code, Host IPC, Go daemon)
        |
        | CodeBridge-owned protocols:
        | ApprovalPresenting, SecretStoring, Updating, AgentDetecting, ProjectPicking
        v
Adapters (CodeBridge code)
        |
        v
Upstream-derived modules (unmodified where possible)
```

Rules:

- upstream types never cross into CodeBridge domain code or Host IPC;
- CodeBridge domain types never enter upstream code;
- upstream code never registers or routes CodeBridge MCP tools and never owns a CodeBridge session model.

### 4.5 Extension points to request upstream

Target: zero. At most:

1. a menu/section provider hook;
2. an approval component with an external decision callback;
3. configurable Keychain service name / access group;
4. configurable updater feed.

Never requested: hooks into upstream's MCP/tool registry or session model.

### 4.6 Sync procedure

- Record repository, commit, license, modules used and reuse mode in an `UPSTREAM.md` next to the upstream-derived code.
- Sync in dedicated commits, never mixed with feature work.
- Review every upstream diff as a security change: this code runs with CodeBridge.app's TCC grants.
- Build and smoke-test after each sync.

### 4.7 Identity

CodeBridge.app has its own bundle id and signing identity. TCC grants are never shared with an upstream app.

## 5. Repository target shape

Conceptual; the build may require a different layout. Dependency direction matters more than folder names.

```text
codebridge/
├── cmd/
│   ├── codebridged/          # V2 daemon: Bridge kernel + domain services + Runtime
│   ├── manager/  client/     # V1, frozen
│   └── doctor/
├── internal/
│   ├── bridge/               # kernel: ingress, caller, capability registry, dispatch
│   ├── policy/  project/  secret/
│   ├── runtime/              # sessions, runs, journal, artifacts (lifted from agentops)
│   ├── agent/                # AgentProviders: omp, codex, opencode
│   ├── computer/             # ComputerSession broker; Host IPC client of the engine
│   ├── hostipc/              # Host IPC bindings
│   └── agentops/ manager/ …  # V1, frozen
├── schema/hostipc/v1/        # language-neutral Host IPC schema
├── desktop/macos/            # CodeBridge.app: UI, computer engine, media, adapters
│   └── ThirdParty/           # upstream-derived code + UPSTREAM.md
└── docs/v2/
```

V2 packages may import lifted V1 code during transition; V1 packages never import V2 packages.

## 6. Migration stages

Aligned with the [Roadmap](roadmap.md).

| Stage | Content |
| --- | --- |
| A — Baseline (Phase 0) | pin or reject upstream; spikes (ingress, lifecycle, TCC attribution, native host, widget media); Host IPC v1 and store schema v1 |
| B — Foundation (Phase 1A) | `codebridged`, Bridge kernel, Policy v2, V2 store and journal, tunnel supervision |
| C — Computer (Phase 1B–2) | computer engine, tools, E2E; then preview and remote takeover |
| D — Runtime (Phase 3) | RunManager lift, AgentProviders, ProviderSession, notifications, V1 history import |
| E — Orchestration / cloud (Phases 4–5) | only after Computer and Runtime are stable |

## 7. Data migration

Decision:

- V2 starts a new store in `~/Library/Application Support/CodeBridge/`.
- V1 `runs.db` (in the user cache directory) is never modified. Phase 3 offers a one-time, read-only import into an `imported` Session per workspace.
- No implicit reinterpretation of V1 rows; the import is explicit and versioned.

## 8. Configuration migration

```text
CODEBRIDGE_MANAGER_HOST       -> removed from personal configuration
workspaces / writable flags   -> Projects
bash / agent permission rules -> Policy rules (verbs + selectors); "always" rules re-confirmed locally
subagent_profiles             -> AgentProvider profiles / ModelPolicy
OAuth / enrollment settings   -> not migrated (tunnel is the remote boundary)
```

Secrets are never copied automatically between storage systems.

## 9. Testing strategy

Three levels:

1. **Contract tests** — Bridge kernel, Policy, Runtime journal, Host IPC schema; no real OS or provider.
2. **Provider tests** — fixtures and fakes plus targeted real smokes (harness CLIs; ScreenCaptureKit / CGEvent on a real Mac).
3. **End-to-end** — real ChatGPT through the tunnel on a native host.

Computer safety tests are mandatory and deterministic where possible: stale frame after controller change, geometry change, local-input preemption, protected surface, out-of-scope app, lease expiry, app quit, daemon restart, IPC version mismatch.

Highest-priority E2E:

```text
ChatGPT -> Secure MCP Tunnel -> codebridged -> CodeBridge.app
  -> computer_observe -> computer_action -> computer_observe
```

Second:

```text
ChatGPT -> agent_start -> durable local run -> disconnect / app quit / daemon restart
  -> continue or interrupted + resumable -> reconnect -> replay / final result
```

## 10. Commit strategy

Reviewable, separate commits:

- upstream import or sync (never mixed with features);
- schemas and contracts;
- daemon foundation;
- computer engine;
- computer tools and E2E;
- runtime lift;
- orchestration;
- cloud.

## 11. Rollback rule

Until V2 reaches the Phase 1 exit:

- V1 stays buildable from the frozen checkpoint;
- no destructive deletion of Manager or Client code;
- V2 changes are reversible by branch or commit;
- upstream syncs are isolated.

After V2 proves the replacement path, V1 deployment components are archived or removed in a dedicated cleanup milestone.
