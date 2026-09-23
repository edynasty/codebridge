# Architecture

## Trust boundaries

```text
ChatGPT / MCP client
        |
        | HTTPS, OAuth in production
        v
+------------------------------+
| Manager (public edge)        |
| MCP endpoint                 |
| device registry + router     |
+---------------+--------------+
                |
                | WSS, device credential
                v
+------------------------------+
| Local agent                  |
| allow-listed workspace roots |
| read/search/git-read only    |
+---------------+--------------+
                |
                v
            local code
```

The Manager is a relay and control plane. It must not persist source payloads. The Local Agent is the data plane and is the only component that has filesystem access.

## Request flow

1. The local agent opens an outbound WebSocket to `/agent` and registers a stable device ID plus logical workspace names.
2. ChatGPT discovers tools from `/mcp` using MCP Streamable HTTP.
3. `list_devices` and `list_workspaces` are answered by the Manager.
4. Filesystem/search/git-read calls are routed to the selected online device.
5. The local agent validates the workspace and relative path, performs the bounded read-only operation, and returns the result.
6. The Manager relays the result to the MCP client without writing it to a database.

## v0.1 security invariants

- No arbitrary command execution.
- No file writes.
- No absolute paths from MCP tools.
- No `..` traversal.
- Existing symlink targets must remain within the workspace root.
- Workspace physical paths are not advertised upstream.
- Bounded file and command output.
- `rg` is invoked without a shell and with `--fixed-strings`.
- Git diff disables external diff and text conversion.
- Browser-originated WebSocket connections to `/agent` are rejected.

## Planned production identity model

```text
Human account --OAuth 2.1--> Manager
                              |
                              +-- account_id -> devices

Local agent --one-time enrollment--> Manager
           <-- device credential ----+
           --mTLS/WSS or signed token->
```

The enrollment token in v0.1 is intentionally temporary scaffolding. It should not become the long-term credential design.
