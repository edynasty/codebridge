# Roadmap

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
- [x] Block common sensitive workspace content by default across read/search/discovery/git status/git diff, with explicit local opt-in.
- [ ] Add tenant/account scoping to every Manager lookup.
- [ ] Run an external-provider OAuth + interactive ChatGPT linking flow against a current RFC 8707-compatible provider.

## P1 - coding intelligence

- [ ] `find_symbol`
- [ ] `find_references`
- [ ] `read_symbol`
- [ ] LSP adapters for Java/TypeScript/Go.
- [ ] tree-sitter fallback.
- [ ] repository dependency graph.
- [ ] optional local index/cache stored only on the agent machine.

## P2 - controlled write mode

- [ ] Explicit per-workspace write opt-in.
- [ ] `apply_patch` only; keep arbitrary shell disabled.
- [ ] Mandatory confirmation metadata for modifying tools.
- [ ] Git checkpoint before writes.
- [ ] Diff preview and rollback.
