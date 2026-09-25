# Roadmap

Engineering execution rules: [docs/development-standards.md](docs/development-standards.md)

Engineering quality/CI/security delivery backlog: [docs/development-todo.md](docs/development-todo.md)


## P0 - make the MVP production-safe enough for personal use

- [x] Add OAuth 2.1 resource-server authentication for MCP users (external IdP).
- [x] Publish RFC 9728 protected-resource metadata and OAuth security metadata on tools.
- [x] Verify access-token signature, issuer, audience/resource, expiration, nbf, subject, and scope.
- [x] Add optional OAuth subject allowlist for single-user/private deployments.
- [x] Replace shared enrollment token with one-time enrollment and per-device credentials.
- [x] Persist device identity metadata and credential hashes only; never persist source payloads.
- [x] Add admin API for enrollment, device revoke, and credential rotation.
- [x] Add MCP request/body limits, per-device concurrency limits, WebSocket size limits, and response-size caps.
- [x] Add TLS/WSS deployment example with Caddy; keep /admin off the public reverse proxy.
- [x] Add Manager-generated request IDs and structured metadata-only audit logs.
- [x] Add version-injected cross-platform binary release packaging and checksums.
- [x] Add non-secret local Client JSON config and launchd/systemd user-service examples.
- [x] Add metadata-only deployment doctor for public OAuth/Admin/device checks.
- [x] Add authenticated Doctor smoke using a real external IdP access token without source-content reads.
- [x] Validate authorization-server metadata, PKCE S256, authorization-code support, and advertised scope compatibility in Doctor.
- [x] Document verified OAuth provider compatibility and a current RFC 8707-capable reference setup.
- [x] Add real Manager↔Client WebSocket integration test covering enrollment, tool execution, one-time codes, and credential reconnect.
- [x] Add official MCP SDK Streamable HTTP integration coverage for tool discovery and invocation.
- [x] Add OAuth JWT → MCP → Manager → WebSocket Client end-to-end integration coverage, including OAuth subject audit metadata.
- [x] Block common sensitive workspace content by default across the file tools (`list`, `read`), with explicit local opt-in.
- [x] Add tenant/account scoping to every Manager lookup.
- [ ] Run an external-provider OAuth + interactive ChatGPT linking flow against a current RFC 8707-compatible provider.

## P1 - coding intelligence

Legacy Client helpers only: the Manager no longer advertises the symbol or dependency-graph tools listed here, so none of them are callable over MCP. The Client keeps routing them for older custom-tool wrappers.

- [x] `find_symbol` (legacy client alias; not advertised over MCP)
- [x] `find_references` (legacy client alias; not advertised over MCP)
- [x] `read_symbol` (legacy client alias; not advertised over MCP)
- [x] LSP adapters for Java/TypeScript/Go (opt-in via `CODEBRIDGE_ENABLE_LSP`; falls back to a portable CGO-free parser instead of tree-sitter).
- [x] tree-sitter fallback (implemented as the built-in portable parser; see note above).
- [x] repository dependency graph (Maven multi-module, npm, Go).
- [x] optional local index/cache stored only on the agent machine.

Note: implementation complete with smoke verification; dedicated regression/integration tests for these helpers are still tracked in docs/development-todo.md.

## P2 - controlled write mode

- [x] Explicit per-workspace write opt-in (`CODEBRIDGE_WRITABLE_WORKSPACES` / `writable_workspaces` JSON).
- [x] `apply_patch` only; arbitrary shell remains disabled.
- [x] Mandatory confirmation metadata for modifying tools (preview → confirm flow).
- [x] Local checkpoint before writes (content-addressed blob snapshots in the client state dir, retained and pruned; no git repository required).
- [x] Diff preview and rollback (`rollback_patch` by checkpoint ID).

Note: implementation complete with smoke verification; dedicated regression/integration tests are still tracked in docs/development-todo.md.
