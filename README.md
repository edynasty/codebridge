# CodeBridge

Private bridge from ChatGPT Web / any MCP client to source code on machines you control.

```text
ChatGPT Web / MCP client
        |
        | HTTPS - MCP Streamable HTTP
        v
+---------------------------+
| CodeBridge Manager        |
| /mcp     MCP tools        |
| /agent   WebSocket hub    |
| device registry/router    |
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

## MVP security model

- Read-only tools only. There is **no arbitrary shell tool** and no file-write tool.
- Local paths never need to be exposed to ChatGPT. Agents advertise logical workspace names.
- Every file path is resolved under an allow-listed workspace root; `..` traversal and absolute paths are rejected.
- `search_code` invokes `rg` with argument arrays, not a shell. It falls back to a bounded Go scanner when ripgrep is unavailable.
- File reads are capped at 256 KiB per call.
- Git commands are fixed read-only commands (`status`, `diff`).
- The manager does not persist source code or tool responses.

This MVP uses a shared enrollment token for local agents. Production hardening should replace that with one-time enrollment plus per-device credentials and OAuth for MCP users.

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

- Go 1.25+ (required by the pinned official MCP Go SDK v1.8.0)
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

Terminal 1:

```bash
export CODEBRIDGE_ENROLL_TOKEN='dev-secret'
go run ./cmd/manager
```

Terminal 2:

```bash
export CODEBRIDGE_ENROLL_TOKEN='dev-secret'
export CODEBRIDGE_MANAGER_URL='ws://127.0.0.1:8080/agent'
export CODEBRIDGE_DEVICE_ID='mbp-m1'
export CODEBRIDGE_DEVICE_NAME='MacBook Pro M1'
export CODEBRIDGE_WORKSPACES='pms=/Users/me/code/pms,portal=/Users/me/code/portal'
go run ./cmd/client
```

Health check:

```bash
curl http://127.0.0.1:8080/healthz
```

## Inspect MCP locally

OpenAI recommends testing a Streamable HTTP MCP server with MCP Inspector before connecting ChatGPT:

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

Then add that remote MCP endpoint from ChatGPT's developer/plugin UI available to your account. The exact UI/plan availability can change; use the current OpenAI Developer Mode documentation for the account you are testing.

For initial personal testing, keep the MCP endpoint private by network/access policy where possible. Before public distribution, implement OAuth rather than relying on a static bearer token.

## Production hardening roadmap

1. One-time agent enrollment codes.
2. Per-device keypairs/credentials, rotation and revocation.
3. PostgreSQL device/account registry.
4. OAuth 2.1 / protected-resource metadata for MCP authentication.
5. Tenant isolation (`account_id` on every device and request).
6. Per-tool and per-workspace policy.
7. Request IDs + metadata-only audit log; never log file contents/tool result bodies.
8. Rate limits, concurrency limits and response byte budgets.
9. TLS/WSS only; origin/host validation.
10. Optional LSP / tree-sitter / code graph tools without arbitrary shell access.

## Non-goals for v0.1

- Editing files
- Running arbitrary commands
- Building/deploying projects
- Persistent source-code indexing on the manager
- Uploading repositories to a third-party relay
