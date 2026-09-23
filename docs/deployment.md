# HTTPS / WSS deployment

The recommended production shape is:

```text
Internet
   |
   | 443 / TLS
   v
 Caddy
   |---------------- /mcp
   |---------------- /agent (WebSocket -> WSS externally)
   |---------------- /.well-known/oauth-protected-resource
   |
   X /admin/*  (404 from the public proxy)
   |
 Docker network
   v
 CodeBridge Manager :8080

Host loopback only:
127.0.0.1:8080/admin/*
```

## Prerequisites

- A DNS name such as `codebridge.example.com` pointing to the server.
- TCP 80 and 443 reachable by Caddy for certificate issuance and HTTPS.
- An external OAuth authorization server configured as described in [oauth.md](oauth.md).
- Docker Compose.

## Environment

Create a private deployment environment file outside Git:

```env
CODEBRIDGE_DOMAIN=codebridge.example.com
CODEBRIDGE_ADMIN_TOKEN=<long-random-secret>

CODEBRIDGE_OAUTH_ISSUER=https://auth.example.com
CODEBRIDGE_OAUTH_JWKS_URL=https://auth.example.com/.well-known/jwks.json
CODEBRIDGE_OAUTH_RESOURCE=https://codebridge.example.com
CODEBRIDGE_OAUTH_SCOPE=codebridge.read

CODEBRIDGE_MAX_INFLIGHT_PER_DEVICE=8
CODEBRIDGE_MAX_MCP_INFLIGHT=16
CODEBRIDGE_MAX_MCP_REQUEST_BYTES=1048576
```

Do not commit this file.

## Start

From the `deploy` directory:

```bash
docker compose --env-file .env -f compose.caddy.yml up -d --build
```

Caddy obtains and renews the public TLS certificate automatically.

## Public checks

```bash
curl -i https://codebridge.example.com/healthz
curl -sS https://codebridge.example.com/.well-known/oauth-protected-resource
```

An unauthenticated MCP request should return `401` with a `WWW-Authenticate: Bearer ... resource_metadata=...` challenge.

```bash
curl -i https://codebridge.example.com/mcp
```

The administrative API must not be publicly reachable:

```bash
curl -i https://codebridge.example.com/admin/devices
# expected: 404
```

## Administrative access

The Caddy compose file publishes Manager port 8080 only on host loopback. Run admin commands on the server itself:

```bash
curl -sS http://127.0.0.1:8080/admin/devices \
  -H "Authorization: Bearer $CODEBRIDGE_ADMIN_TOKEN"
```

For remote administration, use SSH port forwarding instead of publishing `/admin`:

```bash
ssh -L 18080:127.0.0.1:8080 user@codebridge-server
```

Then access `http://127.0.0.1:18080/admin/...` locally.

## Multi-account deployments

Devices are grouped into accounts for MCP visibility (see [oauth.md](oauth.md#accounts-and-device-visibility)). The admin bearer token is a deployment-wide operator credential: it can create enrollment codes for any account, rotate and revoke any device, and list all devices with their `account_id`. Keep `/admin` loopback-bound and keep the admin token out of any account owner's hands — account isolation applies to MCP callers, not to the deployment operator.

## Local agents

Local clients connect outbound through the same public domain:

```env
CODEBRIDGE_MANAGER_URL=wss://codebridge.example.com/agent
```

No inbound port is required on the developer laptop. Caddy forwards the WebSocket upgrade to the Manager automatically.

## Reverse-proxy logging

Do not configure a proxy to log request or response bodies. Source contents travel through MCP responses and must remain transient. Normal access logs containing method, path, status, duration, and byte counts are sufficient.

## Firewall

On the server, expose only the ports actually required:

- 22/tcp (or your SSH port) from trusted administration networks;
- 80/tcp for ACME HTTP challenges / redirects;
- 443/tcp and optionally 443/udp for HTTPS/HTTP3.

Do not expose Manager port 8080 publicly.
