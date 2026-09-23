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

- Read-only MCP tools only. There is **no arbitrary shell tool** and no file-write tool.
- Local paths never need to be exposed to ChatGPT. Agents advertise logical workspace names.
- Every file path is resolved under an allow-listed workspace root; `..` traversal, absolute paths, and symlink escapes are rejected.
- `search_code` invokes `rg` with argument arrays, not a shell. It falls back to a bounded Go scanner when ripgrep is unavailable.
- File reads are capped at 256 KiB per call.
- Git commands are fixed read-only commands (`status`, `diff`).
- The manager does not persist source code or tool responses.
- Device enrollment uses short-lived **one-time enrollment codes**.
- Each enrolled device receives a random **per-device credential**. The manager persists only its SHA-256 digest; the client persists the credential locally with file mode `0600`.
- Device credentials can be rotated or revoked through the admin API.
- Runtime safety limits bound MCP request size/concurrency, per-device in-flight calls, WebSocket messages, directory listings, and agent response payloads.
- Every MCP/admin/agent request gets a Manager-generated request ID, and security-relevant actions emit structured metadata-only JSONL audit events.
- MCP user access supports OAuth 2.1 with an external IdP: RFC 9728 protected-resource metadata, JWT signature/issuer/audience/expiry/subject/scope validation, optional subject allowlisting, and OAuth security metadata on each tool.
- CodeBridge deliberately does not implement passwords, login pages, authorization-code issuance, or refresh-token storage; use an established authorization server.

## MCP tools

| Tool | Purpose |
| --- | --- |
| `list_devices` | List connected agents |
| `list_workspaces` | List logical workspaces on a device |
| `list_directory` | List a workspace-relative directory |
| `read_file` | Read a bounded source/text file |
| `find_files` | Find files by glob/path substring |
| `search_code` | Literal code search, preferably via ripgrep |
| `git_status` | Read git status |
| `git_diff` | Read unstaged git diff |
| `project_info` | Detect build/project markers |

All tools are declared `readOnlyHint=true` and `openWorldHint=false`. When OAuth is enabled, each tool also advertises the `codebridge.read` OAuth scope (or your configured scope).

## Requirements

- Go 1.25+
- `git` on each local agent
- `rg` (ripgrep) recommended
- Public HTTPS endpoint for ChatGPT Web, e.g. Caddy/Nginx/Cloudflare Tunnel in front of the manager

The project uses the official `github.com/modelcontextprotocol/go-sdk` and exposes MCP Streamable HTTP at `/mcp`.

## Build

```bash
go mod tidy
make test
make build
```

Both binaries support `--version`. Tagged GitHub releases automatically publish Client/Manager binaries for macOS Intel/Apple Silicon, Linux amd64/arm64, and Windows amd64 with a `SHA256SUMS` file.

See [docs/releases.md](docs/releases.md).

## Local Client configuration

The Client optionally reads `~/.config/codebridge/client.json`, so Manager URL, device identity, and workspace roots do not need to live in shell environment variables. CLI flags override environment variables, which override the JSON configuration.

```json
{
  "manager_url": "wss://codebridge.example.com/agent",
  "device_id": "mbp-m1",
  "device_name": "MacBook Pro",
  "workspaces": {
    "pms": "/Users/me/code/pms"
  }
}
```

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

The enrollment code is single-use and expires automatically.

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
```

The authorization server must issue JWT access tokens with the exact issuer, the configured resource in `aud`, a non-empty `sub`, a valid expiration, and the required scope. When `CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS` is set, only those exact subjects are accepted. It must also support the MCP/ChatGPT OAuth flow (authorization code + PKCE S256 and a compatible client registration/identification method).

CodeBridge exposes:

```text
GET /.well-known/oauth-protected-resource
POST /mcp   Authorization: Bearer <access-token>
```

See [docs/oauth.md](docs/oauth.md), [docs/chatgpt-web.md](docs/chatgpt-web.md), [docs/deployment.md](docs/deployment.md), and [docs/audit.md](docs/audit.md).

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

Deploy the Manager at a stable HTTPS origin and add:

```text
https://codebridge.example.com/mcp
```

as the remote MCP endpoint in ChatGPT's developer/plugin UI. With OAuth configured, ChatGPT can discover the protected-resource metadata and link the user's account before calling the read-only tools.

## HTTPS/WSS deployment

A Caddy + Docker Compose example is included under `deploy/`. It terminates TLS, forwards `/mcp` and `/agent`, and deliberately returns 404 for public `/admin/*`; the Manager admin API is bound to host loopback only.

See [docs/deployment.md](docs/deployment.md).

## Audit logging

By default, audit events are emitted as JSONL to stdout. Set:

```bash
export CODEBRIDGE_AUDIT_LOG='./data/audit.jsonl'
```

to persist them locally with mode `0600`.

Audit events include request ID, OAuth subject, tool name, device ID, workspace, duration, and success/error category. They intentionally exclude source contents, result bodies, paths, search queries/patterns, tokens, enrollment codes, and credentials.

See [docs/audit.md](docs/audit.md).

## Production hardening roadmap

1. PostgreSQL account/device registry for multi-tenant deployment.
2. Tenant isolation (`account_id` on every device and request).
3. Per-tool and per-workspace policy.
4. Run MCP Inspector OAuth flow against a real Auth0/Keycloak/Authentik tenant.
5. Optional LSP / tree-sitter / code graph tools without arbitrary shell access.

## Non-goals for v0.3

- Editing files
- Running arbitrary commands
- Building/deploying projects
- Persistent source-code indexing on the manager
- Uploading repositories to a third-party relay
