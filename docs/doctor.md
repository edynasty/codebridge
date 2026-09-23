# Deployment doctor

`codebridge-doctor` performs metadata-only deployment checks. It never reads a workspace or calls source-code MCP tools.

## Public production check

```bash
codebridge-doctor --url https://codebridge.example.com
```

The default production profile checks:

1. `GET /healthz` returns healthy;
2. RFC 9728 protected-resource metadata is reachable and contains a resource + authorization server;
3. unauthenticated `/mcp` returns `401`;
4. the Bearer challenge contains `resource_metadata=...`;
5. public `/admin/devices` returns `404`.

Example output:

```text
[PASS] health                 HTTP 200
[PASS] oauth_metadata         resource=https://codebridge.example.com authorization_servers=1 scopes=1
[PASS] mcp_auth_challenge     HTTP 401; Bearer resource_metadata="..."
[PASS] public_admin_blocked   HTTP 404
```

The process exits non-zero if any required check fails.

## Check the private Admin API and one device

Run this on the Manager host, or through an SSH tunnel:

```bash
export CODEBRIDGE_ADMIN_TOKEN='...'

codebridge-doctor \
  --url https://codebridge.example.com \
  --admin-url http://127.0.0.1:8080 \
  --device-id mbp-m1
```

The doctor additionally checks:

- Admin bearer authentication;
- `/admin/devices` JSON response;
- whether the requested device exists;
- whether that device is currently online.

The admin token can be passed with `--admin-token`, but the environment variable is preferred so the secret does not appear in the process command line.

## Local/development Manager

For a development Manager without OAuth:

```bash
codebridge-doctor \
  --url http://127.0.0.1:8080 \
  --no-oauth \
  --no-admin-block-check
```

## JSON output

```bash
codebridge-doctor --url https://codebridge.example.com --json
```

Example shape:

```json
{
  "ok": true,
  "results": [
    {
      "name": "health",
      "ok": true,
      "detail": "HTTP 200"
    }
  ]
}
```

This is suitable for deployment smoke tests, CI, or an external monitor.

## What doctor deliberately does not test

The doctor does not:

- obtain an OAuth token;
- perform the interactive ChatGPT OAuth linking flow;
- call `read_file`, `search_code`, or any other source-reading tool;
- reveal device credentials;
- expose workspace paths.

A complete release still needs an actual ChatGPT/IdP MCP smoke test after deployment.
