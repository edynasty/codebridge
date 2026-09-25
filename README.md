# CodeBridge

Self-hosted bridge from ChatGPT Web / any MCP client to source code on machines you control.

```text
ChatGPT Web / MCP client
        |
        | HTTPS - MCP Streamable HTTP
        v
+---------------------------+
| CodeBridge Manager        |
| /mcp     MCP tools        |
| /agent   WebSocket hub    |
| /admin   device identity  |
+-------------^-------------+
              | outbound WSS
        +-----+------+
        | Local Agent |
        | list/read   |
        | bash (gated)|
        +------+------+
               |
       local workspaces
```

## Security model

- Read-only MCP tools by default. File writes (`edit`, `write`, `apply_patch`, `rollback_patch`) are disabled by default and require an explicit **local** per-workspace opt-in (`CODEBRIDGE_WRITABLE_WORKSPACES`) that a remote MCP caller can never enable.
- `bash` and `agent` exist, but neither runs until the **local** operator approves it: an unmatched command or subagent harness returns a permission request id, and `permission_grant` (`once` or a persisted `always` rule) is what releases it. A remote MCP caller can never grant itself permission. Once approved, a subagent runs autonomously inside its workspace, so that permission rule is the only bound on it.
- Local paths never need to be exposed to ChatGPT. Agents advertise logical workspace names.
- Every file path is resolved under an allow-listed workspace root; `..` traversal, absolute paths, and symlink escapes are rejected.
- File reads are capped at 256 KiB per call.
- `bash` runs `bash -c` with the workspace root as the working directory, a 30-second timeout, a 256 KiB combined-output cap, and no TTY. Its output is deliberately **not** run through the sensitive-path filter — the permission gate, not output filtering, is the control — so approve commands with that in mind.
- Common sensitive workspace content such as `.env`, private keys, cloud credentials, `.git` internals, and Terraform state/variables is blocked by default in the file tools (`list`, `read`). Disabling this requires an explicit local Client opt-in. Sensitive paths are **never writable**, even with that opt-in.
- `edit`, `write`, and `apply_patch` are all-or-nothing, require `preview` then `confirm`, always create a local checkpoint before writing, reject binary content and sensitive paths, and can be undone with `rollback_patch`. They never run shell commands and never push.
- The manager does not persist source code or tool responses.
- Device enrollment uses short-lived **one-time enrollment codes**.
- Each enrolled device receives a random **per-device credential**. The manager persists only its SHA-256 digest; the client persists the credential locally with file mode `0600`.
- Device credentials can be rotated or revoked through the admin API.
- Runtime safety limits bound MCP request size/concurrency, per-device in-flight calls, WebSocket messages, directory listings, and agent response payloads.
- Every MCP/admin/agent request gets a Manager-generated request ID, and security-relevant actions emit structured metadata-only JSONL audit events.
- MCP user access supports OAuth 2.1 with an external IdP: RFC 9728 protected-resource metadata, JWT signature/issuer/audience/expiry/subject/scope validation, optional subject allowlisting, and OAuth security metadata on each tool.
- Every device belongs to an account. MCP callers only see and call devices in their own account; wrong-account device IDs behave like unknown ones. See [docs/oauth.md](docs/oauth.md#accounts-and-device-visibility).
- CodeBridge deliberately does not implement passwords, login pages, authorization-code issuance, or refresh-token storage; use an established authorization server.

## MCP tools

Read-only tools are declared `readOnlyHint=true` and `openWorldHint=false`. The mutating tools — `edit`, `write`, `apply_patch`, `rollback_patch`, `bash`, and `agent` — are declared `readOnlyHint=false`/`destructiveHint=true`. When OAuth is enabled, each tool also advertises the `codebridge.read` OAuth scope (or your configured scope).

Every workspace-scoped tool call targets one device and workspace: pass `device_id` (from `list_devices`) and `workspace` (the logical name from `list_workspaces`). Paths are always **workspace-relative**; absolute paths and `..` traversal are rejected, and symlink escapes are contained inside the workspace root.

The local client still routes a few pre-`bash` helper names from older versions, but the manager neither advertises them nor accepts them as custom-tool presets, so they are not part of the public surface.

### Discovery

#### `list_devices`

No arguments. Lists the devices connected to the manager in your account, with `id`, `name`, `account_id`, `version`, `online`, `connected_at`, `last_seen`, and the `workspaces` each device advertises.

#### `list_workspaces`

```json
{ "device_id": "mbp-m1" }
```

Lists one device's logical workspaces: `name`, plus `"writable": true` when the local client opted that workspace into write mode. Only writable workspaces accept `edit`, `write`, `apply_patch`, and `rollback_patch`.

#### `agents_list`

```json
{ "device_id": "mbp-m1" }
```

Lists the local coding-agent profiles available on a device as `{"agents": [...]}`: `name`, `description`, `client`, `agent`, `model`, `thinking`, `timeout_seconds`, and `tab`. Call this before `agent` and pass the chosen `name` as `agent`.

### Files

#### `list`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "path": "src/main/java" }
```

Directory listing like `ls`: `name`, `path`, `type` (`file`, `dir`, or `symlink`), and `size`. `path` is optional and defaults to the workspace root. Sensitive entries are omitted; a directory with more than 1000 entries is rejected, so narrow the path.

#### `read`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "path": "README.md", "max_bytes": 262144 }
```

Returns `path`, `content`, `truncated`, `bytes_read`. Always capped at 256 KiB per call; `max_bytes` narrows the read (larger values are clamped).

### Write mode (opt-in)

The file-write tools (`edit`, `write`, `apply_patch`, `rollback_patch`) only work on workspaces the **local client** explicitly advertised as writable (`CODEBRIDGE_WRITABLE_WORKSPACES` / `writable_workspaces` JSON). A remote MCP caller can never enable this. Any workspace directory works (no git repository required); sensitive paths (`.env`, private keys, …) are never writable; binary content is rejected.

`edit`, `write`, and `apply_patch` are all-or-nothing and two-step: call with `preview: true` to get the diff, then repeat the identical call with `confirm: true` to write. A call that sets neither flag is rejected. Every apply snapshots the affected files into the local content-addressed checkpoint store first and returns a `checkpoint_id`. All three return the same result shape:

```json
{
  "applied": true,
  "files": [{ "path": "src/main/java/.../RefundService.java", "action": "replace" }],
  "checkpoint_id": "cbk_1a2b3c4d5e6f7a8b",
  "message": "Applied 1 file change(s). Roll back with rollback_patch and checkpoint_id cbk_1a2b3c4d5e6f7a8b."
}
```

A preview call returns `"applied": false`, `"preview": true`, the unified `diff`, the per-file `files` plan, and a message saying nothing was written.

#### `edit`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "path": "src/main/java/.../RefundService.java",
  "old_text": "    BigDecimal amount = total;", "new_text": "    BigDecimal amount = applyThreshold(total);",
  "preview": true }
```

Replaces exactly one occurrence of `old_text` with `new_text`; zero matches or more than one match fail the whole call.

#### `write`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "path": "docs/notes.md", "content": "# notes\n", "preview": true }
```

Writes a file's full content, creating it or overwriting it.

#### `apply_patch`

```json
{ "device_id": "mbp-m1", "workspace": "pms",
  "edits": [
    { "path": "src/main/java/.../RefundService.java", "old_text": "    BigDecimal amount = total;", "new_text": "    BigDecimal amount = applyThreshold(total);" },
    { "path": "docs/notes/new.md", "new_text": "# notes\n" }
  ],
  "preview": true }
```

Several edits applied together, all-or-nothing. Each edit is one of:
- **replace** (`old_text` + `new_text`): `old_text` must match exactly once;
- **create** (`new_text` only): the file must not exist;
- **delete** (`old_text` only): `old_text` must equal the full file content.

An edit that sets neither field, a duplicate path, or any single failure aborts the whole patch (files already written are restored from the checkpoint). Bounds: 20 files, 512 KiB per file, 2 MiB total.

#### `rollback_patch`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "checkpoint_id": "cbk_1a2b3c4d5e6f7a8b" }
```

Restores exactly the files touched by that write to their pre-patch content:

```json
{
  "rolled_back": true,
  "checkpoint_id": "cbk_1a2b3c4d5e6f7a8b",
  "restored_files": ["src/main/java/.../RefundService.java"],
  "removed_files": ["docs/notes/new.md"]
}
```

`restored_files` lists files returned to earlier content, `removed_files` the files the patch had created. Checkpoints are kept locally, bounded to the most recent 20.

### Shell (permission-gated)

#### `bash`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "command": "go test ./internal/agentops/..." }
```

Runs `bash -c <command>` with the workspace root as the working directory: 30-second timeout, combined stdout+stderr capped at 256 KiB, no TTY, `TERM=dumb` and `PAGER=cat`. It does not require the write-mode opt-in, but every command passes the local permission gate first. Returns:

```json
{ "command": "go test ./...", "output": "ok  \tgithub.com/edynasty/codebridge/internal/agentops\t0.42s\n", "truncated": false }
```

`error` and `exit_code` are added when the command fails or is killed (a timeout reports `exit_code: -1`). There is no interactive input: stdin is the null device, so a command that prompts reads EOF and fails instead of waiting.

Commands do not run until the **local** operator approves them. The first call for a command that no local rule matches returns a permission request id instead of output:

```text
permission required: call permission_grant with request_id=req_1a2b3c4d5e6f7a8b decision=once|always|deny to approve command "go test ./..."
```

#### `permission_grant`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "request_id": "req_1a2b3c4d5e6f7a8b", "decision": "once" }
```

Resolves one pending request:

- `once` — approves this single execution; retry the blocked call and it runs;
- `always` — persists a rule for the command's first word (so a rule for `git` covers `git status` and `git log`) and the retry runs;
- `deny` — rejects the request.

Pending requests expire after 5 minutes, and a retried blocked call reuses the existing request id instead of minting a new one. Operators can also pre-approve commands without answering prompts, through the client's `permissions` rules (`effect`, `tool`, `pattern`) or `CODEBRIDGE_BASH_PERMISSIONS=full|deny`.

### Subagents

#### `agent`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "agent": "refactor", "task": "Extract the retry logic in internal/agentops into its own file and run the package tests." }
```

Delegates one self-contained task to a local coding-agent subagent that works autonomously inside the workspace. The subagent shares no conversation context, so `task` must stand on its own. Inputs: `task` (required), `client` (`omp`, the default, or `opencode`/`codex`), `agent` (a profile name from `agents_list`), `model` (`provider/model`), `thinking` (`off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`), and `timeout_seconds` (0 or omitted means no wall-clock limit). At most 2 subagents run concurrently per client, and live subagent events are streamed to the MCP caller as progress notifications.

Returns `client`, `agent`, `model`, `thinking`, `task`, `status` (`completed`, `failed`, or `timeout`), `output` (the final assistant message plus a bounded raw event tail), `events` (JSONL event count), and `elapsed`.

The harness is permission-gated like `bash`: the client needs an allow rule for it (for example an `agent` rule for `omp`), otherwise the first call returns a permission request id for `permission_grant`. `agent` does not require the write-mode opt-in, and an approved harness runs autonomously (`--auto-approve` / `--auto`) with the workspace as its working directory, so the permission rule — not the write opt-in — is what bounds it.

## Requirements

- Go 1.26+
- `git` on each local agent, only needed if you allow git commands through `bash`
- `rg` (ripgrep) recommended, for shell searches you allow through `bash`
- Public HTTPS endpoint for ChatGPT Web, e.g. Caddy/Nginx/Cloudflare Tunnel in front of the manager

The project uses the official `github.com/modelcontextprotocol/go-sdk` and exposes MCP Streamable HTTP at `/mcp`.

## Development standards

All human and AI contributors must follow [docs/development-standards.md](docs/development-standards.md).

Use:

- [TODO.md](TODO.md) for product capability milestones;
- [docs/development-todo.md](docs/development-todo.md) for engineering quality, CI, security regression, integration, and delivery work.

A non-trivial task starts with a Todo/acceptance checklist and is complete only after implementation, tests, documentation/config synchronization, and the final commit's CI are green.

## Build

```bash
go mod tidy
make test
make build
```

Client, Manager, and Doctor support `--version`. Tagged GitHub releases automatically publish all three binaries for macOS Intel/Apple Silicon, Linux amd64/arm64, and Windows amd64 with a `SHA256SUMS` file.

See [docs/releases.md](docs/releases.md).

## Local Client configuration

The Client optionally reads `~/.config/codebridge/client.json`, so Manager URL, device identity, and workspace roots do not need to live in shell environment variables. CLI flags override environment variables, which override the JSON configuration.

```json
{
  "manager_host": "codebridge.example.com:8081",
  "device_id": "mbp-m1",
  "device_name": "MacBook Pro",
  "allow_sensitive_files": false,
  "workspaces": {
    "pms": "/Users/me/code/pms"
  },
  "writable_workspaces": ["pms"],
  "enable_lsp": true
}
```

`writable_workspaces` is the local write opt-in for `edit`/`write`/`apply_patch`/`rollback_patch`; the same list can be set with `CODEBRIDGE_WRITABLE_WORKSPACES`. `enable_lsp` (or `CODEBRIDGE_ENABLE_LSP=true`) only affects the legacy symbol helpers the client still routes for older configurations, making them prefer locally installed language servers (`gopls`, `typescript-language-server --stdio`, `jdtls`) and fall back to the built-in portable parser; it does not change the advertised tool surface. Local symbol-index and write-checkpoint caches live under `~/.cache/codebridge/` (override with `CODEBRIDGE_STATE_DIR`/`CODEBRIDGE_INDEX_DIR`) and are never uploaded.

Device credentials remain in the separate `credentials.json` file with mode `0600`; enrollment codes and credentials should not be put in `client.json`.

macOS launchd and Linux systemd user-service examples are included. See [docs/client.md](docs/client.md).

## Run locally

### 1. Start the Manager

```bash
export CODEBRIDGE_ADMIN_TOKEN='replace-with-a-long-random-admin-token'
export CODEBRIDGE_STATE_FILE='./data/auth.json'
go run ./cmd/manager
```

`CODEBRIDGE_STATE_FILE` stores device metadata, enrollment-code hashes, and device-credential hashes. It never stores source-code payloads.

### 2. Create a one-time enrollment code

```bash
curl -sS -X POST http://127.0.0.1:8080/admin/enrollments \
  -H 'Authorization: Bearer replace-with-a-long-random-admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"ttl_seconds":600}'
```

Example response:

```json
{
  "enrollment_code": "enr_...",
  "expires_at": "2026-09-23T04:20:00Z",
  "expires_in": 599
}
```

The enrollment code is single-use and expires automatically. The enrolled device joins the `default` account unless you pass an explicit `"account_id"` (typically your OAuth subject for a personal deployment, or a shared account name for a team).

### 3. Start a Local Agent once with the enrollment code

```bash
export CODEBRIDGE_MANAGER_HOST='127.0.0.1:8081'
export CODEBRIDGE_DEVICE_ID='mbp-m1'
export CODEBRIDGE_DEVICE_NAME='MacBook Pro M1'
export CODEBRIDGE_WORKSPACES='pms=/Users/me/code/pms,portal=/Users/me/code/portal'
export CODEBRIDGE_ENROLL_CODE='enr_...'
go run ./cmd/client
```

After successful enrollment, the client writes its device credential to:

```text
~/.config/codebridge/credentials.json
```

with file mode `0600`. Remove `CODEBRIDGE_ENROLL_CODE`; future reconnects use the saved device credential.

### 4. List enrolled devices

```bash
curl -sS http://127.0.0.1:8080/admin/devices \
  -H 'Authorization: Bearer replace-with-a-long-random-admin-token'
```

Each entry includes the device's `account_id` and online state. The admin API is deployment-wide by design; MCP callers only ever see their own account's devices.

### Rotate a device credential

```bash
curl -sS -X POST http://127.0.0.1:8080/admin/devices/mbp-m1/rotate \
  -H 'Authorization: Bearer replace-with-a-long-random-admin-token'
```

The new credential is returned once. Update the local client with `CODEBRIDGE_DEVICE_CREDENTIAL` before its next reconnect; the client will persist the override into its credential file.

### Revoke a device

```bash
curl -sS -X DELETE http://127.0.0.1:8080/admin/devices/mbp-m1 \
  -H 'Authorization: Bearer replace-with-a-long-random-admin-token'
```

Revocation also disconnects the currently active WebSocket. Re-enrollment requires a fresh one-time enrollment code.

## OAuth for ChatGPT Web

For private repositories, configure an external OAuth 2.1 authorization server:

```bash
export CODEBRIDGE_PUBLIC_URL='https://codebridge.example.com'
export CODEBRIDGE_OAUTH_ISSUER='https://auth.example.com'
export CODEBRIDGE_OAUTH_JWKS_URL='https://auth.example.com/.well-known/jwks.json'
export CODEBRIDGE_OAUTH_RESOURCE='https://codebridge.example.com'
export CODEBRIDGE_OAUTH_SCOPE='codebridge.read'
# Recommended for a private/personal deployment after you know your IdP subject:
export CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS='your-oauth-subject'
# Optional: bind subjects to a shared account. Unmapped subjects are their own
# account, so enroll devices with account_id=<subject> (or map subjects to the
# account your devices use, e.g. "default" for migrated single-user state).
# export CODEBRIDGE_OAUTH_ACCOUNT_MAP='your-oauth-subject=default'
```

The authorization server must issue JWT access tokens with the exact issuer, the configured resource in `aud`, a non-empty `sub`, a valid expiration, and the required scope. When `CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS` is set, only those exact subjects are accepted. It must also support the MCP/ChatGPT OAuth flow (authorization code + PKCE S256 and a compatible client registration/identification method).

CodeBridge exposes:

```text
GET /.well-known/oauth-protected-resource
POST /mcp   Authorization: Bearer <access-token>
```

See [docs/oauth.md](docs/oauth.md), [docs/oauth-providers.md](docs/oauth-providers.md), [docs/chatgpt-web.md](docs/chatgpt-web.md), [docs/deployment.md](docs/deployment.md), and [docs/audit.md](docs/audit.md).

## Inspect MCP locally

For a local no-OAuth development run:

```bash
npx @modelcontextprotocol/inspector
```

Use:

```text
http://127.0.0.1:8080/mcp
```

Do not expose a no-auth MCP endpoint to the public internet.

## Connect ChatGPT Web

Deploy the Manager at a stable HTTPS origin and register the full MCP endpoint in ChatGPT developer mode:

```text
https://codebridge.example.com/mcp
```

With OAuth configured, ChatGPT discovers the protected-resource metadata and links the user's account before calling the read-only tools.

A developer-mode MCP connection and an installed plugin package are separate layers. If you want a reusable CodeBridge plugin package, copy the registered connection's `plugin_asdk_app...` technical ID and generate the package:

```bash
make plugin-web \
  MCP_URL=https://codebridge.example.com/mcp \
  APP_ID=plugin_asdk_app_0123456789abcdef \
  PLUGIN_OUT=dist/codebridge-plugin.zip
```

The generated package contains portable `plugin.json` / `mcp.json`, Codex compatibility files, and the ChatGPT `.app.json` mapping. The public endpoint and registered app ID are generated locally and are not committed to the repository.

If `@CodeBridge` is visible but the current chat has no CodeBridge Tools, verify the MCP server with Doctor first, then check the registered MCP connection, generated `.app.json` mapping, plugin installation, and a fresh chat. Do not duplicate Tool registration in the Manager.

See [docs/chatgpt-web.md](docs/chatgpt-web.md) for the complete registration, packaging, installation, and troubleshooting flow.

## HTTPS/WSS deployment

A Caddy + Docker Compose example is included under `deploy/`. It terminates TLS, forwards `/mcp` and `/agent`, and deliberately returns 404 for public `/admin/*`; the Manager admin API is bound to host loopback only.

See [docs/deployment.md](docs/deployment.md).

## Deployment doctor

`codebridge-doctor` performs metadata-only deployment checks for HTTPS health, OAuth protected-resource discovery, the unauthenticated MCP Bearer challenge, public `/admin` blocking, and optionally private Admin/device-online state.

```bash
codebridge-doctor --url https://codebridge.example.com
```

If `CODEBRIDGE_ACCESS_TOKEN` is set, Doctor also performs an authenticated MCP handshake and `list_devices` call with the real token. With `--device-id` and `--workspace`, it can additionally cross the live Agent path with a read-only `list` call on the workspace root.

See [docs/doctor.md](docs/doctor.md).

## Audit logging

By default, audit events are emitted as JSONL to stdout. Set:

```bash
export CODEBRIDGE_AUDIT_LOG='./data/audit.jsonl'
```

to persist them locally with mode `0600`.

Audit events include request ID, OAuth subject, tool name, device ID, workspace, duration, and success/error category. They intentionally exclude source contents, result bodies, paths, search queries/patterns, tokens, enrollment codes, and credentials.

See [docs/audit.md](docs/audit.md).

## Integration coverage

CI now exercises three real protocol layers:

- Manager ↔ Client WebSocket enrollment, one-time enrollment-code consumption, tool round-trip, disconnect cleanup, and credential reconnect;
- official MCP Go SDK Streamable HTTP discovery and tool invocation;
- signed JWT/JWKS OAuth authentication through `/mcp`, then `read` across Manager → WebSocket Client → local workspace, including OAuth `sub` propagation into metadata-only audit records.

The remaining OAuth gap is an external-provider/UI smoke test against a provider that implements the current MCP requirements, plus the interactive ChatGPT linking flow. The provider compatibility guide currently uses WorkOS AuthKit as the reference path because its documentation explicitly covers RFC 8707 Resource Indicators; Keycloak 26.7.x is not treated as fully compatible because its own MCP guide says Resource Indicators are not supported.

## Production hardening roadmap

1. PostgreSQL account/device registry for large multi-tenant deployment (account scoping itself is implemented on the JSON state file).
2. Run an external-provider OAuth flow and interactive ChatGPT linking test against a current RFC 8707-compatible provider.
3. Per-tool and per-workspace policy.
4. Optional LSP / tree-sitter / code graph tools without arbitrary shell access.

## Non-goals for v0.3

- Arbitrary file writes outside the structured, opt-in `apply_patch` flow
- Running arbitrary commands
- Building/deploying projects
- Persistent source-code indexing on the manager
- Uploading repositories to a third-party relay
