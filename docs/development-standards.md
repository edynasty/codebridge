# CodeBridge Development Standards

This document is the engineering contract for human and AI contributors. It defines the rules that must remain true while CodeBridge evolves.

The product roadmap lives in [../TODO.md](../TODO.md). Engineering-governance work lives in [development-todo.md](development-todo.md).

## 1. Core principles

1. **Local source stays local.** The Manager may relay bounded tool results but must not persist source files, search results, diffs, or tool response bodies.
2. **Read-only by default.** New MCP capabilities are read-only unless a future write-mode milestone explicitly changes that boundary.
3. **No arbitrary remote shell.** Do not add a generic shell/command execution tool.
4. **Logical workspace boundary.** Remote callers operate on logical workspace names and workspace-relative paths, never physical host paths.
5. **Fail closed.** Authentication, path validation, sensitive-file checks, limits, and policy errors must reject on uncertainty.
6. **Secure defaults, local opt-in for exceptions.** A remote MCP caller must not be able to weaken local Client security policy.
7. **Every bug fix gets a regression test.** Security fixes require a test that proves the previous bypass no longer works.
8. **A task is not complete until CI is green and documentation/Todo state is synchronized.**

## 2. Repository responsibilities

### `cmd/manager`

Only process startup, configuration loading, dependency wiring, and HTTP/WebSocket server assembly belong here.

Business/security behavior belongs under `internal/`.

### `cmd/client`

Only Client startup, local configuration, credential loading, signal handling, and session wiring belong here.

Workspace operations and path/security policy belong under `internal/agentops`.

### `cmd/doctor`

Doctor is a deployment diagnostic tool. It may validate metadata, authentication, protocol connectivity, device visibility, and metadata-only Agent round trips.

Doctor must not become a general source-reading client.

### `internal/manager`

Owns:

- device registry and connection lifecycle;
- MCP tool routing;
- WebSocket Agent handling;
- admin endpoints;
- Manager-side limits and concurrency;
- metadata-only audit integration.

It must not implement local filesystem access.

### `internal/agentops`

Owns all workspace filesystem/search/git operations and the local security boundary.

Any new source-reading capability must pass through this layer rather than bypassing it from `cmd/client`.

### Authentication packages

OAuth validation, device credential persistence, enrollment, and authorization metadata remain separate from tool business logic. Do not mix token parsing into individual MCP tools.

## 3. Architecture invariants

The following are **non-negotiable invariants** unless an explicit design change is approved and documented.

### Manager invariants

- Public MCP endpoint is `/mcp`.
- Agent transport is outbound Client → Manager WebSocket.
- Public deployment must use HTTPS/WSS.
- Public reverse proxy must not expose `/admin/*`.
- Manager does not persist source-code/tool-result payloads.
- Audit records contain metadata only.
- Pending MCP calls fail promptly when an Agent disconnects/reconnects.
- Agent connections have bounded concurrency, message sizes, and stale-session handling.

### Client invariants

- Workspaces are explicitly allow-listed.
- Remote input never supplies or changes a physical workspace root.
- Absolute paths, `..` traversal, symlink escape, and path-race bypasses must be rejected.
- Common sensitive files/directories are blocked by default.
- Sensitive-file protection can only be disabled through explicit **local** configuration/environment/CLI.
- Device credentials are stored separately from non-secret Client configuration.
- Remote Manager URLs require WSS except explicitly local development endpoints.

### OAuth invariants

- CodeBridge is an OAuth protected resource, not an authorization server.
- Access-token signature, issuer, audience/resource, lifetime, subject, and scope are verified.
- Audience/resource verification must not be weakened to accommodate a provider.
- Current ChatGPT/MCP OAuth requirements must be verified against current provider documentation before provider-specific guidance is changed.
- OAuth failures fail closed.
- Tokens, authorization codes, refresh tokens, enrollment codes, and device credentials must never appear in logs or audit payloads.

## 4. Task and Todo protocol

Every non-trivial development task starts with a Todo list before editing code.

Use stable IDs:

- `SEC-xxx` — security/auth/path/privacy work;
- `MCP-xxx` — MCP protocol/tools;
- `AGT-xxx` — local Agent/Client;
- `MGR-xxx` — Manager/runtime;
- `OPS-xxx` — deployment/release/observability;
- `INT-xxx` — integration/external-provider testing;
- `DX-xxx` — development tooling/governance;
- `CI-xxx` — CI/build/test infrastructure.

Example:

```text
SEC-014 Sensitive symlink policy
- [ ] define expected behavior
- [ ] implement path validation
- [ ] add direct-read regression test
- [ ] add search/discovery regression test
- [ ] run full CI
- [ ] update security/client docs
```

Rules:

- One Todo item must describe one verifiable outcome.
- Do not mark an item complete because code was written; mark it complete only after its acceptance condition passes.
- If implementation discovers additional required work, add a new Todo before doing it.
- Do not silently broaden scope.
- Open product work belongs in root `TODO.md`.
- Engineering-process work belongs in `docs/development-todo.md`.
- Temporary scratch Todos must be removed or promoted to one of those files before task completion.

## 5. Change workflow

For each task:

1. Read this document, relevant architecture/security docs, and the applicable Todo.
2. Identify affected trust boundaries and public interfaces.
3. Write or update the Todo/acceptance criteria.
4. Inspect existing tests before implementation.
5. Make the smallest coherent change.
6. Add tests in the same task.
7. Run required local validation.
8. Update configuration examples/docs when behavior or configuration changes.
9. Check the diff for secret/path leakage and accidental scope expansion.
10. Commit atomically.
11. Confirm CI passes.
12. Update Todo state only after validation.

Do not stack unrelated fixes into the same logical task. If an unrelated defect is found, create a separate Todo and separate commit unless it is required to make the current change correct.

## 6. Go coding rules

- Target Go version follows `go.mod` and CI.
- Run `gofmt`; formatting is not optional.
- Prefer standard library primitives over custom wrappers when they improve security or portability.
- Keep functions focused; split policy decisions from transport/wiring.
- Errors returned to MCP callers must be useful but must not expose physical workspace roots, credentials, tokens, provider secrets, or internal stack traces.
- Wrap errors only when the wrapped message is safe to expose.
- Avoid package-global mutable state.
- Bound memory and output for all externally triggered operations.
- Use `context.Context` for network, search, git, and potentially long-running operations.
- Every spawned goroutine must have an explicit shutdown/cancellation path.
- Never invoke user-controlled strings through a shell.
- External commands must use argument arrays and fixed executable names.
- Do not add CGO dependencies without an explicit portability decision.

## 7. MCP Tool rules

Every MCP Tool must define:

- clear purpose;
- explicit JSON input schema;
- bounded input/output;
- device/workspace authorization path;
- read/write classification;
- failure behavior;
- audit metadata;
- tests.

Current default annotations:

- `readOnlyHint=true`;
- `openWorldHint=false`.

A new Tool must not:

- accept a raw host filesystem path;
- accept a raw shell command;
- bypass `internal/agentops` for local source access;
- return physical workspace roots;
- return secret/config credential content by default;
- persist result bodies on the Manager.

For tools that accept `device_id` or `workspace`, validate both through the normal Manager/Agent routing path. Do not create alternate lookup paths that bypass future tenant scoping.

## 8. Filesystem and sensitive-content rules

All file operations must satisfy both containment and sensitivity policy.

Mandatory checks:

- reject absolute paths;
- reject lexical traversal;
- reject symlink escape;
- re-check sensitive policy after symlink/real-path resolution;
- resist path replacement/race where practical using root-relative OS APIs;
- return logical paths only;
- hide sensitive file names from discovery/status surfaces where policy requires;
- do not follow sensitive directory aliases indirectly;
- preserve safe example/template files intentionally allowed by policy.

When adding a new operation that enumerates, reads, diffs, searches, hashes, parses, or indexes files, add a security test proving sensitive files are not exposed through that new surface.

## 9. Authentication and authorization rules

### MCP OAuth

A protected MCP request must not reach a Tool until bearer validation succeeds.

Tests must cover relevant combinations of:

- valid token;
- invalid signature;
- wrong issuer;
- wrong audience/resource;
- expired token;
- future `nbf`;
- missing subject;
- denied subject allowlist;
- missing required scope.

### Agent identity

- Enrollment codes are short-lived and one-time.
- Long-lived authentication uses per-device credentials.
- Only credential digests are persisted by the Manager.
- Revocation must terminate the active connection.
- Rotation must invalidate the old credential.
- Registration requests are size/rate/concurrency bounded.

### Admin

- Admin API is private/loopback by deployment design.
- Admin bearer authentication is still required.
- Never rely on network placement as the only admin authorization control.

## 10. Limits and denial-of-service controls

Any new externally reachable operation must answer:

- maximum request bytes;
- maximum response bytes;
- maximum number of returned items;
- maximum runtime/deadline;
- maximum per-device/global concurrency;
- behavior on disconnect/cancellation.

Do not introduce unbounded:

- file reads;
- directory walks;
- search output;
- WebSocket payloads;
- goroutines;
- pending request maps;
- HTTP bodies;
- connection counts.

## 11. Testing standard

### Unit tests

Required for:

- parsers/validators;
- policy decisions;
- path/security helpers;
- configuration precedence;
- error cases and boundary values.

### Regression tests

Every fixed defect must include a test that fails against the old behavior.

Security bugs require a specifically named regression test describing the bypass.

### Integration tests

Maintain these layers:

1. Manager ↔ Client WebSocket enrollment and reconnect;
2. official MCP Go SDK Streamable HTTP discovery/tool call;
3. OAuth JWT/JWKS → MCP → Manager → WebSocket Client;
4. deployment Doctor metadata/authenticated smoke.

When changing protocol boundaries, extend the relevant real integration test rather than relying only on mocks.

### Cross-platform expectations

All Client, Manager, and Doctor code must compile for:

- macOS amd64;
- macOS arm64;
- Linux amd64;
- Linux arm64;
- Windows amd64.

OS-specific filesystem behavior must have explicit tests/skips with a reason.

## 12. Required validation before commit

At minimum:

```bash
make fmt-check
go test ./...
go vet ./...
make build
```

For security/concurrency changes, also run when practical:

```bash
go test -race ./...
```

The GitHub CI matrix is the authoritative cross-platform compile gate.

A commit is not considered finished until the CI run for that final commit is green.

## 13. Commit standard

Use atomic, intent-based commits.

Preferred message style:

```text
Add authenticated MCP integration coverage
Protect sensitive workspace content by default
Limit concurrent agent websocket connections
```

Rules:

- imperative/intent-first subject;
- one logical change per final commit;
- tests and directly related documentation may be in the same commit;
- do not leave formatting-only or temporary repair commits in final history when they are part of the same task;
- squash exploratory/fixup commits before declaring the task complete;
- never commit secrets, tokens, real credentials, private keys, user source code, or local absolute paths;
- do not force-push shared history when other contributors may be building on it; use task branches/PRs for shared development.

## 14. Documentation synchronization

Update documentation in the same task when any of these change:

| Change | Required documentation |
| --- | --- |
| environment/config option | `.env.example`, relevant docs, example JSON if applicable |
| MCP Tool/input/output | README tool table + relevant MCP docs/tests |
| authentication behavior | `docs/oauth.md`, provider guide if relevant, Doctor behavior |
| Client behavior | `docs/client.md`, examples |
| deployment/network behavior | `docs/deployment.md` |
| audit fields/semantics | `docs/audit.md` |
| roadmap state | root `TODO.md` |
| engineering process | this document + `development-todo.md` |

Documentation must describe current behavior, not intended future behavior.

## 15. Configuration rules

- Environment variables use `CODEBRIDGE_` prefix.
- Defaults must be safe for production unless explicitly documented as local-development-only.
- Secrets should prefer environment/file sources that do not expose them in process arguments.
- JSON Client config is for non-secret configuration.
- Device credentials remain in the dedicated credential store.
- New boolean security relaxations default to `false`.
- Configuration precedence must be documented and tested.

## 16. Observability and audit rules

Audit security-relevant actions with:

- request ID;
- actor subject where applicable;
- action/tool;
- device ID;
- logical workspace;
- duration;
- success/failure category.

Never audit:

- source content;
- tool result bodies;
- search strings when they may contain code/secrets;
- physical workspace paths;
- access/device/admin tokens;
- enrollment codes;
- credentials.

Logs should remain useful for diagnosing lifecycle and protocol failures without violating the same privacy boundary as MCP responses.

## 17. AI-agent contribution rules

AI agents working on this repository must:

1. read this document and relevant Todo before editing;
2. preserve existing architecture/security invariants;
3. verify current external protocol/provider facts from authoritative documentation before changing compatibility guidance;
4. not weaken validation to make a test/provider pass;
5. add tests with implementation, not later;
6. avoid unrelated refactors during a focused task;
7. never introduce a generic shell Tool;
8. never expose local absolute paths or secrets in examples/tests/logs;
9. stop when acceptance criteria are met and final CI is green;
10. leave a concise record of completed and remaining Todo items.

If an AI agent detects a security issue while implementing another feature, it should create a separate security Todo immediately. It may fix it in the same work session when necessary, but the final history should keep the security change coherent and reviewable.

## 18. Definition of Done

A task is **Done** only when all applicable boxes are true:

- [ ] Todo/acceptance criteria exist.
- [ ] Implementation is complete.
- [ ] No architecture invariant was weakened.
- [ ] Unit/regression tests were added or updated.
- [ ] Relevant integration test passes.
- [ ] Security/privacy impact was checked.
- [ ] Limits/cancellation were considered.
- [ ] `make fmt-check` passes.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] `make build` passes.
- [ ] Cross-platform CI is green.
- [ ] Configuration examples are synchronized.
- [ ] Documentation is synchronized.
- [ ] Product/engineering Todo state is synchronized.
- [ ] Final commit history is coherent and contains no fixup noise.
