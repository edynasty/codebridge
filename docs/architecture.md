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
| device identity + router     |
+---------------+--------------+
                |
                | WSS, per-device credential
                v
+------------------------------+
| Local agent                  |
| allow-listed workspace roots |
| read-only tools by default   |
+---------------+--------------+
                |
                v
            local code
```

The Manager is a relay and control plane. It must not persist source payloads. The Local Agent is the data plane and is the only component that has filesystem access.

## Device identity lifecycle

```text
Administrator
    |
    | POST /admin/enrollments
    v
one-time enrollment code (short TTL)
    |
    v
Local Agent ---- WSS register ----> Manager
    |                                  |
    |<-- per-device credential --------+
    |
    +-- stores credential locally (0600)

Future reconnects:
Local Agent ---- device credential ---> Manager
```

Manager persistence contains only:

- device ID/name, owning account ID, and timestamps;
- SHA-256 digests of high-entropy device credentials;
- SHA-256 digests of still-valid one-time enrollment codes.

Raw device credentials are returned only at enrollment or explicit rotation and are not persisted by the Manager.

## Accounts and tenancy

Every device record carries an `account_id`. The account is set at enrollment from the enrollment code's `account_id` (default: `default`) and is bound to the online registry entry by the Manager from persisted state — never from agent-supplied input, so a device cannot relabel itself into another account.

MCP requests are scoped to the caller's account:

- with OAuth enabled, the verified `sub` is resolved through `CODEBRIDGE_OAUTH_ACCOUNT_MAP`; unmapped subjects are their own account;
- with OAuth disabled (local development), requests run under the `default` account;
- `list_devices`, `list_workspaces`, and every device tool call match only devices in the caller's account, and wrong-account device IDs are indistinguishable from unknown ones.

The admin API remains deployment-wide (admin token + loopback placement) and shows each device's `account_id`.

## Request flow

1. The local agent opens an outbound WebSocket to `/agent` and authenticates using a saved device credential, or uses a one-time enrollment code on first registration.
2. ChatGPT discovers tools from `/mcp` using MCP Streamable HTTP.
3. `list_devices` and `list_workspaces` are answered by the Manager from the caller's account.
4. File and shell tool calls are routed to the selected online device, but only if that device belongs to the caller's account.
5. The local agent validates the workspace and relative path, performs the bounded operation, and returns the result.
6. The Manager relays the result to the MCP client without writing it to a database or source cache.

## v0.2 security invariants

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
- Enrollment codes are single-use and short-lived.
- Device credentials are high-entropy and stored only as hashes on the Manager.
- Admin API is disabled unless `CODEBRIDGE_ADMIN_TOKEN` is configured.
- Every Manager device lookup is scoped by account; a device's account always comes from persisted state, never from agent input.

## Next identity layer

```text
Human account --OAuth 2.1--> Manager
                               |
                               +-- account_id -> devices

Local agent --device credential--> Manager
```

OAuth is now the identity boundary for `/mcp`; device credentials remain separate from human/MCP credentials. Account scoping is implemented on the current JSON state file; a PostgreSQL multi-account registry remains a future deployment option.
