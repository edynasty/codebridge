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
        | read/search |
        | git read    |
        +------+------+
               |
       local workspaces
```

## Security model

- Read-only MCP tools by default. There is **no arbitrary shell tool**. File writes exist only as the structured `apply_patch` tool, are disabled by default, and require an explicit **local** per-workspace opt-in (`CODEBRIDGE_WRITABLE_WORKSPACES`) that a remote MCP caller can never enable.
- Local paths never need to be exposed to ChatGPT. Agents advertise logical workspace names.
- Every file path is resolved under an allow-listed workspace root; `..` traversal, absolute paths, and symlink escapes are rejected.
- `search_code` invokes `rg` with argument arrays, not a shell. It falls back to a bounded Go scanner when ripgrep is unavailable.
- File reads are capped at 256 KiB per call.
- Git commands are fixed read-only commands (`status`, `diff`). Sensitive paths are filtered from status and diff output by default.
- Common sensitive workspace content such as `.env`, private keys, cloud credentials, `.git` internals, and Terraform state/variables is blocked by default across read/search/discovery tools. Disabling this requires an explicit local Client opt-in. Sensitive paths are **never writable**, even with that opt-in.
- `apply_patch` is all-or-nothing, requires `preview` then `confirm`, always creates a git checkpoint before writing, rejects binary content and sensitive paths, and can be undone with `rollback_patch`. It never runs shell commands and never pushes.
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

All tools are declared `readOnlyHint=true` and `openWorldHint=false`, except `apply_patch`/`rollback_patch`, which are declared mutating. When OAuth is enabled, each tool also advertises the `codebridge.read` OAuth scope (or your configured scope).

Every tool call targets one device and workspace: pass `device_id` (from `list_devices`) and `workspace` (the logical name from `list_workspaces`). Paths are always **workspace-relative**; absolute paths and `..` traversal are rejected, and symlink escapes are contained inside the workspace root.

### Discovery

#### `list_devices`

Lists currently connected devices with `id`, `name`, `account_id`, `online`, and their advertised workspaces.

#### `list_workspaces`

```json
{ "device_id": "mbp-m1" }
```

Lists the logical workspaces of one device. Entries with `"writable": true` accept `apply_patch` (local opt-in on the client).

#### `list_directory`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "path": "src/main/java" }
```

Directory entries (`name`, `path`, `type`, `size`). Defaults to the workspace root.

### Reading

#### `read_file`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "path": "README.md", "max_bytes": 262144 }
```

Returns `path`, `content`, `truncated`, `bytes_read`. Capped at 256 KiB per call; use `max_bytes` to narrow.

#### `find_files`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "pattern": "*Service*" }
```

Filename glob or path substring, bounded to 500 results. Skips `node_modules`, `target`, `dist`, `.git`, etc.

#### `search_code`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "query": "processRefund", "path": "src" }
```

Literal (non-regex) text search, via ripgrep when installed, otherwise a bounded built-in scanner. Up to 200 matching lines with `path`, `line`, `text`.

#### `git_status` / `git_diff`

```json
{ "device_id": "mbp-m1", "workspace": "pms" }
{ "device_id": "mbp-m1", "workspace": "pms", "path": "src/main/java" }
```

Read-only `git status --short --branch` and unstaged diff, optionally scoped to one path. Sensitive files are filtered out by default.

#### `project_info`

```json
{ "device_id": "mbp-m1", "workspace": "pms" }
```

Detected build markers (`pom.xml`, `go.mod`, `package.json`, `Dockerfile`, …) and whether the workspace is a git repository.

### Coding intelligence

#### `find_symbol`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "pattern": "RefundService", "kind": "class" }
```

Symbol declarations by name substring. Optional `kind` filter: `function`, `method`, `class`, `interface`, `struct`, `type`, `enum`, `const`, `var`. Returns `name`, `kind`, `path`, `line`, `end_line`, `signature`, `container` (enclosing type for methods), `language` — supports Go, Java, TypeScript/JavaScript. Bounded to 200 results.

#### `read_symbol`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "path": "src/main/java/.../RefundService.java", "symbol": "processRefund", "max_lines": 512 }
```

Returns the smallest useful declaration range of one symbol (`symbol` metadata + `content` + `truncated`). Use `find_symbol` first when you only know the name.

#### `find_references`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "symbol": "processRefund", "path": "src/main/java/.../RefundService.java" }
```

References to a symbol as `{path, line}` hits. `path` (the declaring file) enables language-server precision when the client has `CODEBRIDGE_ENABLE_LSP=true`; otherwise bounded word-boundary matching is used. The response reports which `engine` answered (`lsp` or `textual`).

#### `dependency_graph`

```json
{ "device_id": "mbp-m1", "workspace": "pms" }
```

Module/dependency graph parsed from `pom.xml` (multi-module Maven), `package.json` (npm) or `go.mod` (Go). Nodes are `{id, path, type, version}`, edges `{from, to, scope, version}`; graph size is bounded. When several manifests exist, Maven wins, then npm, then Go.

### Write mode (opt-in)

Write tools only work on workspaces the **local client** explicitly advertised as writable (`CODEBRIDGE_WRITABLE_WORKSPACES` / `writable_workspaces` JSON). A remote MCP caller can never enable this. The workspace must be a git repository; sensitive paths (`.env`, private keys, …) are never writable; binary content is rejected.

#### `apply_patch`

```json
{ "device_id": "mbp-m1", "workspace": "pms",
  "edits": [
    { "path": "src/main/java/.../RefundService.java", "old_text": "    BigDecimal amount = total;", "new_text": "    BigDecimal amount = applyThreshold(total);" },
    { "path": "docs/notes/new.md", "new_text": "# notes\n" }
  ],
  "preview": true }
```

All-or-nothing edits. Each edit is one of:
- **replace** (`old_text` + `new_text`): `old_text` must match exactly once;
- **create** (`new_text` only): the file must not exist;
- **delete** (`old_text` only): `old_text` must equal the full file content.

Always call with `preview: true` first — the response contains a unified diff, the per-file plan, and nothing is written. Then send the same patch with `confirm: true` to apply. Every apply snapshots the affected files as git blobs first and returns a `checkpoint_id`. Bounds: 20 files, 512 KiB per file, 2 MiB total.

#### `rollback_patch`

```json
{ "device_id": "mbp-m1", "workspace": "pms", "checkpoint_id": "cbk_1a2b3c4d5e6f7a8b" }
```

Restores exactly the files touched by that `apply_patch` to their pre-patch content (restored files and removed created-files are reported). Checkpoints are kept locally, bounded to the most recent 20.

## Requirements

- Go 1.25+
- `git` on each local agent
- `rg` (ripgrep) recommended
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
  "manager_url": "wss://codebridge.example.com/agent",
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

`writable_workspaces` is the local write opt-in for `apply_patch`/`rollback_patch`; the same list can be set with `CODEBRIDGE_WRITABLE_WORKSPACES`. `enable_lsp` (or `CODEBRIDGE_ENABLE_LSP=true`) makes the symbol tools prefer locally installed language servers (`gopls`, `typescript-language-server --stdio`, `jdtls`), falling back to the built-in portable parser. Local symbol-index and write-checkpoint caches live under `~/.cache/codebridge/` (override with `CODEBRIDGE_STATE_DIR`/`CODEBRIDGE_INDEX_DIR`) and are never uploaded.

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
export CODEBRIDGE_MANAGER_URL='ws://127.0.0.1:8080/agent'
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

If `CODEBRIDGE_ACCESS_TOKEN` is set, Doctor also performs an authenticated MCP handshake and `list_devices` call with the real token. With `--device-id` and `--workspace`, it can additionally cross the live Agent path using metadata-only `project_info`.

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
- signed JWT/JWKS OAuth authentication through `/mcp`, then `read_file` across Manager → WebSocket Client → local workspace, including OAuth `sub` propagation into metadata-only audit records.

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
