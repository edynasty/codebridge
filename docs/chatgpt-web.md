# ChatGPT Web connection

CodeBridge exposes a standard MCP Streamable HTTP endpoint at `/mcp`.

## Production OAuth prerequisites

For private source code, configure CodeBridge as an OAuth 2.1 protected resource and use an established authorization server such as Auth0, Keycloak, or Authentik.

CodeBridge needs:

```env
CODEBRIDGE_PUBLIC_URL=https://codebridge.example.com
CODEBRIDGE_OAUTH_ISSUER=https://auth.example.com
CODEBRIDGE_OAUTH_JWKS_URL=https://auth.example.com/.well-known/jwks.json
CODEBRIDGE_OAUTH_RESOURCE=https://codebridge.example.com
CODEBRIDGE_OAUTH_SCOPE=codebridge.read
```

The authorization server must:

- publish OAuth/OIDC discovery metadata;
- support authorization-code + PKCE S256;
- accept ChatGPT using CIMD, DCR, or a pre-registered OAuth client;
- preserve the OAuth `resource` parameter and mint the configured resource as the access-token audience;
- issue signed JWT access tokens whose keys are available from the configured JWKS URL;
- include the required `codebridge.read` scope (or your configured scope).

CodeBridge publishes:

```text
GET /.well-known/oauth-protected-resource
GET /.well-known/oauth-protected-resource/mcp
```

Unauthenticated `/mcp` requests return a Bearer challenge pointing to the protected-resource metadata. Every MCP request then validates the JWT signature, issuer, audience, expiration/not-before claims, and scope.

## Development flow

1. Start the Manager and at least one Local Agent.
2. For local-only testing you may omit OAuth and validate `http://localhost:8080/mcp` with MCP Inspector. Do not expose a no-auth MCP endpoint publicly.
3. For OAuth testing, configure a development IdP tenant and the environment variables above.
4. Put the Manager behind HTTPS, for example `https://codebridge.example.com/mcp`.
5. In ChatGPT, enable **Settings -> Security and login -> Developer mode** when available to the current account/workspace.
6. Open **ChatGPT Plugins**, add an MCP connection, and enter the full `https://codebridge.example.com/mcp` URL.
7. ChatGPT discovers the protected-resource metadata and opens account linking when authentication is required.
8. Switch to Work and invoke the plugin with `@`.

Current OpenAI docs:
- https://developers.openai.com/plugins/quickstart
- https://developers.openai.com/plugins/build/mcp-server
- https://developers.openai.com/plugins/build/auth

## Suggested first prompts

```text
@CodeBridge list my connected devices and workspaces.
```

```text
@CodeBridge inspect the pms workspace on mbp-m1. Read pom.xml, list the top-level modules, and summarize the project structure. Do not modify anything.
```

```text
@CodeBridge search the pms workspace for AuthorizationServerConfigurerAdapter and show the matching files and line numbers.
```

## Development bearer fallback

If OAuth is not configured, `CODEBRIDGE_MCP_TOKEN` can protect `/mcp` for MCP clients that can send a static bearer token. This is a development compatibility option, not the ChatGPT account-linking design.
