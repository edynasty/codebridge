# Deployment doctor

`codebridge-doctor` performs metadata-only deployment checks. It never reads a workspace or calls source-code MCP tools.

## Public production check

```bash
codebridge-doctor --url https://codebridge.example.com
```

The default production profile checks:

1. `GET /healthz` returns healthy;
2. RFC 9728 protected-resource metadata is reachable and contains a resource + authorization server;
3. each advertised authorization server exposes usable OAuth/OIDC metadata;
4. authorization-code endpoints and PKCE `S256` are advertised;
5. CodeBridge resource scopes are compatible with the authorization server's advertised scopes when that list is present;
6. unauthenticated `/mcp` returns `401`;
7. the Bearer challenge contains `resource_metadata=...`;
8. public `/admin/devices` returns `404`.

Example output:

```text
[PASS] health                 HTTP 200
[PASS] oauth_metadata         resource=https://codebridge.example.com authorization_servers=1 scopes=1
[PASS] authorization_server   issuer=https://auth.example.com pkce=S256 refresh=true dcr=true scopes=4
[PASS] mcp_auth_challenge     HTTP 401; Bearer resource_metadata="..."
[PASS] public_admin_blocked   HTTP 404
```

The process exits non-zero if any required check fails.

## Authenticated MCP smoke with a real access token

When you have a real access token from Auth0, Keycloak, Authentik, or another configured authorization server, export it only for the Doctor process:

```bash
export CODEBRIDGE_ACCESS_TOKEN='eyJ...'

codebridge-doctor --url https://codebridge.example.com
```

When `CODEBRIDGE_ACCESS_TOKEN` is present, Doctor additionally uses the official MCP Go client to:

- authenticate to `/mcp` with the real bearer token;
- complete the MCP initialization handshake;
- list tools;
- invoke `list_devices`.

The token is intentionally environment-only; there is no `--access-token` flag, so the bearer token does not need to appear in the process command line.

To also verify a live Agent without reading source contents:

```bash
CODEBRIDGE_ACCESS_TOKEN='eyJ...' codebridge-doctor \
  --url https://codebridge.example.com \
  --device-id mbp-m1 \
  --workspace pms
```

This additionally calls `list_workspaces` and the read-only `project_info` tool. `project_info` crosses the real Manager → WebSocket Client path but only returns project markers such as `pom.xml`, `go.mod`, or `package.json`; it does not return file contents.

A successful run separates two classes of failure:

- if authenticated Doctor fails, investigate the IdP token, issuer/audience/scope/subject mapping, CodeBridge OAuth configuration, or MCP endpoint;
- if authenticated Doctor succeeds but ChatGPT linking fails, investigate the external authorization-code/PKCE/client-registration/UI flow.

Unset the token after the test:

```bash
unset CODEBRIDGE_ACCESS_TOKEN
```

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

- obtain an OAuth token for you;
- perform the interactive ChatGPT OAuth linking flow;
- call `read_file`, `search_code`, or any other source-content tool;
- reveal device credentials;
- expose workspace paths.

A complete release still needs the interactive ChatGPT/IdP linking flow after deployment. The authenticated Doctor smoke can validate a real IdP-issued access token independently of that UI flow.
