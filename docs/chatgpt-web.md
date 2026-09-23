# ChatGPT Web connection

CodeBridge exposes a standard MCP Streamable HTTP endpoint at `/mcp`.

## Development flow

1. Start the Manager and at least one Local Agent.
2. Validate `http://localhost:8080/mcp` with MCP Inspector.
3. Put the Manager behind an HTTPS endpoint, for example `https://codebridge.example.com/mcp`.
4. In ChatGPT, enable **Settings -> Security and login -> Developer mode** when that feature is available to the current account/workspace.
5. Open **ChatGPT Plugins**, add an MCP connection, and enter the full `/mcp` URL.
6. Create/install the personal plugin, switch to Work, and invoke it with `@`.

Current OpenAI docs:
- https://developers.openai.com/plugins/quickstart
- https://developers.openai.com/plugins/build/mcp-server
- https://developers.openai.com/plugins/deploy/connect-chatgpt

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

## Authentication note

The current MVP can expose `/mcp` without application authentication for a tightly controlled development endpoint, or use an optional static bearer token for non-ChatGPT MCP clients that can send it.

Before public use, implement the MCP OAuth flow. Do not expose a no-auth MCP endpoint containing private source-code access to the open internet.
