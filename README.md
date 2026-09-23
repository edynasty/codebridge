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

MCP user authentication is still P0 work. Keep `/mcp` behind HTTPS/access controls until OAuth is implemented.

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

All tools are declared `readOnlyHint=true` and `openWorldHint=false`.

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

## Inspect MCP locally

```bash
npx @modelcontextprotocol/inspector
```

Use:

```text
http://127.0.0.1:8080/mcp
```

## Connect ChatGPT Web

Put `/mcp` behind a stable HTTPS endpoint such as:

```text
https://codebridge.example.com/mcp
```

Then add that remote MCP endpoint from ChatGPT's developer/plugin UI available to your account.

For initial personal testing, keep the MCP endpoint private by network/access policy where possible. Before public distribution, implement OAuth rather than relying on an unauthenticated or static-token MCP endpoint.

## Production hardening roadmap

1. OAuth 2.1 / protected-resource metadata for MCP user authentication.
2. PostgreSQL account/device registry for multi-tenant deployment.
3. Tenant isolation (`account_id` on every device and request).
4. Per-tool and per-workspace policy.
5. Request IDs + metadata-only audit log; never log file contents/tool result bodies.
6. Rate limits, concurrency limits and response byte budgets.
7. TLS/WSS only; origin/host validation.
8. Optional LSP / tree-sitter / code graph tools without arbitrary shell access.

## Non-goals for v0.2

- Editing files
- Running arbitrary commands
- Building/deploying projects
- Persistent source-code indexing on the manager
- Uploading repositories to a third-party relay
