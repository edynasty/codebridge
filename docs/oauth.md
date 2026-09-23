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
CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS=
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

### `CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS`

Optional comma-separated allowlist of exact OAuth `sub` values.

All accepted access tokens must contain a non-empty `sub`. When the allowlist is empty, any authenticated subject with the required audience/scope is allowed. When it is configured, only listed subjects are accepted.

For a personal deployment, setting this after the first successful OAuth login is recommended:

```env
CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS=auth0|abc123
```

Do not use email addresses unless your IdP intentionally uses a stable email as `sub`; normally the opaque immutable subject identifier is safer.

### `CODEBRIDGE_OAUTH_ACCOUNT_MAP`

Optional comma-separated binding of OAuth subjects to shared accounts, formatted as `subject=account`:

```env
CODEBRIDGE_OAUTH_ACCOUNT_MAP=auth0|abc123=team-a,auth0|def456=team-a
```

Subjects not listed in the map resolve to their own account (the subject string itself), which is the fail-closed default: no subject can reach another tenant's devices unless an operator explicitly maps it. Subjects mapped to the same account share visibility of that account's devices.

Devices join an account at enrollment time via the admin `account_id` field; for a personal deployment, create enrollment codes with `account_id` equal to your OAuth subject (or map your subject to the `default` account where pre-existing devices live).

## Accounts and device visibility

Every MCP tool call is scoped to the caller's account:

1. the verified token `sub` identifies the subject;
2. `CODEBRIDGE_OAUTH_ACCOUNT_MAP` (if configured) resolves the subject to an account; unmapped subjects are their own account;
3. `list_devices`, `list_workspaces`, and every device tool call only match devices enrolled in that account;
4. wrong-account device IDs return the same error as unknown device IDs, so account membership cannot be probed.

When OAuth is disabled (local development), all MCP requests run under the `default` account, and enrollment codes without an explicit `account_id` enroll devices into that same account — so a no-OAuth local setup behaves as a single tenant.

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
8. a non-empty `sub` is present;
9. if configured, `sub` is in `CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS`;
10. required scope is present in `scope` or `scp`.

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
