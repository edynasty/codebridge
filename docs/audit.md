# Audit logging

CodeBridge emits one JSON object per line for security-relevant Manager events.

By default audit events go to stdout. To persist them:

```env
CODEBRIDGE_AUDIT_LOG=/data/audit.jsonl
```

The file is created with mode `0600`.

## Recorded fields

Depending on the event, CodeBridge records only metadata such as:

- timestamp;
- event name;
- Manager-generated request ID;
- OAuth subject (`actor_id`) when available;
- MCP tool name;
- logical device ID;
- logical workspace name;
- success/failure;
- duration in milliseconds;
- controlled error category.

Example:

```json
{"time":"2026-09-23T04:30:00Z","event":"mcp.tool","request_id":"...","actor_id":"user-123","tool":"read","device_id":"mbp-m1","workspace":"pms","success":true,"duration_ms":18}
```

## Explicitly not recorded

Audit events never include:

- access tokens or refresh tokens;
- admin tokens;
- enrollment codes;
- device credentials;
- source file content;
- MCP result bodies;
- file paths;
- search queries;
- search patterns;
- raw tool argument objects.

This is intentional. A path, query, or result can itself contain confidential source information.

## Event names

Current events include:

- `mcp.tool`
- `admin.enrollment.create`
- `admin.device.rotate`
- `admin.device.revoke`
- `device.enroll`
- `device.connect`
- `device.disconnect`

## Correlation IDs

The Manager generates a fresh request ID for every incoming `/mcp`, `/agent`, and `/admin` request. Client-provided request IDs are overwritten.

For HTTP requests the same ID is returned as:

```text
X-Request-ID: <id>
```

MCP tool audit records use that same ID, allowing an operator to correlate the access log and the tool audit line without logging request bodies.

## Log shipping

Ship the JSONL file or stdout stream to your own logging system if needed. Keep the destination private; although source code is excluded, device IDs, workspace names, and OAuth subjects are operational metadata.
