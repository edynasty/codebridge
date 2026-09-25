# ChatGPT Web connection

CodeBridge exposes a standard MCP Streamable HTTP endpoint at `/mcp`.

There are **two separate integration layers**:

1. **Remote MCP connection** — ChatGPT developer mode connects directly to the CodeBridge `/mcp` endpoint and discovers Tools such as `list_devices` and `list_workspaces`.
2. **Installed plugin package** — a ChatGPT/Codex plugin package maps the registered MCP connection into the plugin through `.app.json`.

Seeing a `@CodeBridge` label or plugin reference does not by itself prove that the current chat has loaded the CodeBridge Tool namespace. The Manager can be healthy and advertise all Tools while a chat still has no callable CodeBridge Tools because the registered MCP connection was not mapped into the installed plugin package or the package was not loaded into that chat.

## Production OAuth prerequisites

For private source code, configure CodeBridge as an OAuth 2.1 protected resource and use an authorization server that satisfies the current ChatGPT/MCP requirements documented in [oauth-providers.md](oauth-providers.md).

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
- accept ChatGPT using a currently supported client registration/identification method;
- preserve the OAuth `resource` parameter and mint the configured resource as the access-token audience;
- issue signed JWT access tokens whose keys are available from the configured JWKS URL;
- include the required `codebridge.read` scope (or your configured scope).

CodeBridge publishes:

```text
GET /.well-known/oauth-protected-resource
GET /.well-known/oauth-protected-resource/mcp
```

Unauthenticated `/mcp` requests return a Bearer challenge pointing to the protected-resource metadata. Every protected MCP request validates the JWT signature, issuer, audience, expiration/not-before claims, subject, and scope.

## Verify the MCP server first

Before diagnosing plugin packaging, prove that the server itself works.

1. Start the Manager and at least one Local Agent.
2. For local-only testing, validate `http://localhost:8080/mcp` with MCP Inspector. Do not expose a no-auth endpoint publicly.
3. For production, put the Manager behind HTTPS and configure OAuth.
4. Run deployment Doctor:

```bash
codebridge-doctor --url https://codebridge.example.com
```

5. With `CODEBRIDGE_ACCESS_TOKEN` set, Doctor also performs an authenticated MCP handshake and calls `list_devices`.
6. The repository integration test `TestStreamableHTTPWithOfficialMCPClient` independently verifies that the official MCP client can discover and call `list_devices`.

If these checks pass but the current ChatGPT conversation has no CodeBridge Tool namespace, investigate the ChatGPT connection/package layer rather than changing `ToolService.Register`.

## Register the MCP connection in ChatGPT

1. Open ChatGPT.
2. Open **Settings -> Security and login** and enable **Developer mode** when available to the current account/workspace.
3. Open **ChatGPT Plugins** and add a remote MCP connection.
4. Enter the full endpoint, for example:

```text
https://codebridge.example.com/mcp
```

5. Complete account linking when OAuth is enabled.
6. After ChatGPT creates the connection, copy its technical ID from the browser URL. The developer-mode ID begins with `plugin_asdk_app`.

A direct developer-mode MCP connection is useful for protocol testing. For a reusable plugin that reliably carries the MCP dependency into a new chat, build and install a plugin package as described below.

## Build a ChatGPT + Codex plugin package

CodeBridge includes `tools/pluginpack`. It generates the current portable Agent Plugins files plus the Codex compatibility files from one MCP endpoint.

After registering the MCP server in ChatGPT and obtaining its `plugin_asdk_app...` technical ID, run:

```bash
make plugin-web \
  MCP_URL=https://codebridge.example.com/mcp \
  APP_ID=plugin_asdk_app_0123456789abcdef \
  PLUGIN_OUT=dist/codebridge-plugin.zip
```

The ZIP contains:

```text
plugin.json
mcp.json
.mcp.json
.app.json
.codex-plugin/plugin.json
```

The generator deliberately keeps the public MCP URL and account/workspace-specific ChatGPT app ID out of the repository. It also normalizes the developer-mode `plugin_asdk_app_...` identifier to the `asdk_app_...` form required inside `.app.json`.

For a portable/Codex package without a registered ChatGPT app mapping:

```bash
make plugin \
  MCP_URL=https://codebridge.example.com/mcp \
  PLUGIN_OUT=dist/codebridge-plugin.zip
```

That package intentionally omits `.app.json` and does **not** claim to be a complete ChatGPT Web registered-app package.

## Install and test the packaged plugin

Use the generated ZIP with the ChatGPT plugin-creator/local plugin workflow for the account or workspace that owns the registered MCP connection. After installation:

1. Refresh ChatGPT so the new plugin release is visible.
2. Start a **new chat** and select/invoke CodeBridge.
3. Verify the CodeBridge Tool namespace is present.
4. Call `list_devices`.
5. Call `list_workspaces` with one returned device ID.
6. Only after those metadata calls succeed, test `project_info`, `read_file`, or `search_code`.

If you update the MCP registration or package mapping, rebuild/reinstall the package rather than assuming an existing chat will hot-reload it.

## Diagnosing “plugin is visible but tools are missing”

Use this order:

1. **Server discovery:** official MCP client/Doctor can list CodeBridge Tools.
2. **Authentication:** the ChatGPT-linked access token has the expected resource/audience and `codebridge.read` scope.
3. **Registered MCP connection:** ChatGPT developer mode shows the intended CodeBridge endpoint.
4. **Plugin mapping:** generated `.app.json` contains the registered server mapping and `plugin.json` / `.codex-plugin/plugin.json` point to `./.app.json`.
5. **Plugin installation:** the generated package is installed for the same user/workspace that owns or may use the registered MCP connection.
6. **Conversation reload:** refresh and test from a new chat.

Do not “fix” this symptom by adding duplicate MCP Tool registrations or weakening OAuth validation. The Manager already registers `list_devices`, `list_workspaces`, and the other read-only Tools through the official Go MCP SDK.

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

## Current OpenAI references

- https://developers.openai.com/plugins/quickstart
- https://developers.openai.com/plugins/build/mcp-server
- https://developers.openai.com/plugins/build/plugins
- https://developers.openai.com/plugins/build/auth
- https://developers.openai.com/plugins/deploy/submission-errors
