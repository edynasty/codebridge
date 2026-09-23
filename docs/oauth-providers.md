# OAuth provider compatibility

CodeBridge is an OAuth protected resource. ChatGPT MCP linking requires more than generic OAuth/OIDC support: the authorization server must be compatible with the current MCP authorization profile.

## Required capabilities

Verify all of the following before treating a provider as compatible:

- OAuth authorization-code flow;
- PKCE with `S256`;
- RFC 8414 authorization-server metadata or compatible OpenID Connect discovery;
- RFC 8707 Resource Indicators: the OAuth client sends `resource` during authorization and token exchange, and the resulting access token is bound to that resource, normally through `aud`;
- a ChatGPT-compatible client identification/registration path: CIMD, DCR, or a predefined client;
- signed access tokens whose `iss`, `aud`, `exp`, `sub`, and required scopes CodeBridge can verify;
- preferably refresh-token support for long-lived linked sessions.

OpenAI reference:

- https://developers.openai.com/plugins/build/auth

## Current provider notes

### WorkOS AuthKit / Connect

WorkOS is the reference configuration documented here because its current MCP documentation explicitly covers:

- Client ID Metadata Document (CIMD);
- optional Dynamic Client Registration (DCR);
- PKCE `S256`;
- RFC 8707 Resource Indicators;
- resource-bound access-token `aud`;
- JWT/JWKS verification.

References:

- https://workos.com/docs/authkit/mcp
- https://workos.com/docs/reference/authkit/oauth-resource
- https://workos.com/docs/reference/workos-connect/metadata
- https://workos.com/docs/authkit/connect/token-claims

### Auth0

OpenAI's current authentication guide includes Auth0 as a supported provider path for MCP authorization. Use Auth0's current MCP-specific guide rather than assuming an older API-audience configuration is sufficient.

Before production use, run `codebridge-doctor` against the configured deployment and confirm that the real access token is resource-bound to the exact CodeBridge resource.

### Keycloak 26.7.x

Do **not** use Keycloak 26.7.x as the reference provider for the latest ChatGPT flow.

Keycloak's current MCP guide explicitly reports RFC 8707 Resource Indicators as unsupported and its current MCP conformance as partial for the newer MCP versions.

Keycloak can still be useful for local/partial protocol experiments, but an audience mapper or scope workaround is not equivalent to correctly processing the MCP-required `resource` parameter.

Reference:

- https://www.keycloak.org/securing-apps/mcp-authz-server

### ZITADEL

Current ZITADEL documentation says Resource Indicators are not yet supported for the relevant OAuth path.

Reference:

- https://zitadel.com/docs/guides/integrate/token-exchange

## WorkOS reference configuration

### 1. Enable MCP client identification

In WorkOS Dashboard, enable **Client ID Metadata Document (CIMD)** under **Connect → Configuration**.

DCR can also be enabled for backwards compatibility.

### 2. Add the CodeBridge Resource Indicator

Register the exact CodeBridge protected-resource URI, for example:

```text
https://codebridge.example.com
```

Use the same value everywhere:

- WorkOS Resource Indicator;
- `CODEBRIDGE_OAUTH_RESOURCE`;
- CodeBridge protected-resource metadata `resource`;
- access-token `aud`.

Do not use the origin in one place and `/mcp` in another.

### 3. Configure CodeBridge

For an AuthKit domain such as `https://example.authkit.app`:

```env
CODEBRIDGE_PUBLIC_URL=https://codebridge.example.com
CODEBRIDGE_OAUTH_ISSUER=https://example.authkit.app
CODEBRIDGE_OAUTH_JWKS_URL=https://example.authkit.app/oauth2/jwks
CODEBRIDGE_OAUTH_RESOURCE=https://codebridge.example.com
CODEBRIDGE_OAUTH_SCOPE=openid
CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS=
```

The reference profile uses `openid` because WorkOS documents that clients identified through CIMD or registered through DCR receive the standard OpenID Connect scopes by default: `openid`, `profile`, `email`, and `offline_access`.

If your WorkOS environment is explicitly configured to grant a custom `codebridge.read` scope to dynamic/CIMD clients, you may use that instead. Do not configure CodeBridge to require a scope the authorization server will refuse to grant.

For a private single-user deployment, the main authorization boundary remains:

- exact issuer;
- exact resource-bound audience;
- verified JWT signature and lifetime;
- non-empty stable user subject;
- optional exact subject allowlist;
- read-only CodeBridge tools;
- local sensitive-file protection.

After the first successful user flow, set `CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS` to the exact stable user `sub`.

### 4. Run preflight checks

```bash
codebridge-doctor --url https://codebridge.example.com
```

Doctor follows CodeBridge protected-resource metadata to the authorization server and validates:

- discovery metadata;
- authorization and token endpoints;
- authorization-code support;
- PKCE `S256`;
- advertised scope compatibility.

### 5. Validate a real token independently of ChatGPT UI

If you already have a user access token issued for the CodeBridge resource:

```bash
CODEBRIDGE_ACCESS_TOKEN='eyJ...' \
  codebridge-doctor --url https://codebridge.example.com
```

To traverse the live Agent path without reading source contents:

```bash
CODEBRIDGE_ACCESS_TOKEN='eyJ...' \
  codebridge-doctor \
    --url https://codebridge.example.com \
    --device-id mbp-m1 \
    --workspace pms
```

A successful authenticated Doctor run proves the token and MCP resource-server path independently of ChatGPT's interactive linking UI.

## Provider acceptance checklist

Before recommending another provider, verify:

1. authorization requests accept `resource=<CodeBridge resource URI>`;
2. token requests preserve the same resource binding;
3. access-token `aud` contains the exact configured CodeBridge resource;
4. PKCE `S256` is advertised;
5. ChatGPT can identify/register through CIMD, DCR, or a predefined client;
6. the user token has a stable non-empty `sub`;
7. the required CodeBridge scope can actually be granted;
8. refresh/reauthorization behavior is compatible with a long-lived linked connection;
9. `codebridge-doctor` succeeds with a real issued token.
