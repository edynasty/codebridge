# OAuth 2.1 resource-server setup

CodeBridge does **not** implement user login or an OAuth authorization server. It is an OAuth protected resource that validates access tokens issued by an established identity provider.

This keeps authentication responsibilities separated:

```text
ChatGPT
   |
   | authorization-code + PKCE
   v
External IdP / Authorization Server
   |
   | JWT access token
   v
CodeBridge /mcp
   |
   | authenticated MCP call
   v
registered local device
```

## CodeBridge settings

```env
CODEBRIDGE_PUBLIC_URL=https://codebridge.example.com
CODEBRIDGE_OAUTH_ISSUER=https://auth.example.com
CODEBRIDGE_OAUTH_JWKS_URL=https://auth.example.com/.well-known/jwks.json
CODEBRIDGE_OAUTH_RESOURCE=https://codebridge.example.com
CODEBRIDGE_OAUTH_SCOPE=codebridge.read
```

### `CODEBRIDGE_PUBLIC_URL`

The externally reachable origin of the Manager. Use HTTPS in production and do not include `/mcp`.

### `CODEBRIDGE_OAUTH_ISSUER`

The authorization server's exact issuer identifier. CodeBridge compares the JWT `iss` claim exactly; do not silently add or remove a trailing slash.

### `CODEBRIDGE_OAUTH_JWKS_URL`

JWKS containing the public signing keys for access tokens. CodeBridge supports RSA and EC signing keys and accepts RS/PS/ES JWT algorithms. Keys are cached for 15 minutes. A stale cache fails closed if it cannot be refreshed.

### `CODEBRIDGE_OAUTH_RESOURCE`

The canonical protected-resource identifier. ChatGPT sends this as the OAuth `resource` parameter. Configure your authorization server to issue the same value in the access-token `aud` claim.

If omitted, CodeBridge uses `CODEBRIDGE_PUBLIC_URL`.

### `CODEBRIDGE_OAUTH_SCOPE`

Required scope for all current read-only tools. Default: `codebridge.read`.

## Protected resource metadata

With OAuth enabled:

```bash
curl https://codebridge.example.com/.well-known/oauth-protected-resource
```

returns metadata similar to:

```json
{
  "resource": "https://codebridge.example.com",
  "authorization_servers": ["https://auth.example.com"],
  "scopes_supported": ["codebridge.read"],
  "bearer_methods_supported": ["header"],
  "resource_name": "CodeBridge"
}
```

CodeBridge also serves the path-form discovery endpoint:

```text
/.well-known/oauth-protected-resource/mcp
```

## Token validation

Every protected MCP request must pass all of these checks:

1. recognized asymmetric signing algorithm (RS, PS, or ES);
2. `kid` resolves to a signing key in the configured JWKS;
3. valid JWT signature;
4. exact issuer match;
5. expected audience/resource is present;
6. `exp` exists and is valid;
7. `nbf`, when present, is valid;
8. required scope is present in `scope` or `scp`.

The MCP middleware also returns a `WWW-Authenticate` challenge containing the resource-metadata URL and required scope.

## ChatGPT OAuth client

Your authorization server must be compatible with the current OpenAI plugin OAuth requirements: authorization-code flow, PKCE S256, resource indicators, and a supported client registration/identification method (CIMD, DCR, or pre-registration).

Prefer CIMD when the provider supports it. Use the exact ChatGPT callback URL and client metadata identifier shown by the ChatGPT plugin management UI for your connection.

## Provider notes

### Auth0

Configure a protected API whose audience equals `CODEBRIDGE_OAUTH_RESOURCE`, enable the `codebridge.read` permission, and use the tenant's exact issuer and JWKS URL.

### Keycloak / Authentik

Configure a client/resource audience mapper so the access token contains `CODEBRIDGE_OAUTH_RESOURCE` in `aud`, and map the read permission into `scope` or `scp`.

Do not weaken CodeBridge audience verification just to accommodate a provider default. Fix the provider's token mapping instead.
