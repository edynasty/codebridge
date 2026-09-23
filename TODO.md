# Roadmap

## P0 - make the MVP production-safe enough for personal use

- [x] Add OAuth 2.1 resource-server authentication for MCP users (external IdP).
- [x] Publish RFC 9728 protected-resource metadata and OAuth security metadata on tools.
- [x] Verify access-token signature, issuer, audience/resource, expiration, nbf, and scope.
- [x] Replace shared enrollment token with one-time enrollment and per-device credentials.
- [x] Persist device identity metadata and credential hashes only; never persist source payloads.
- [x] Add admin API for enrollment, device revoke, and credential rotation.
- [x] Add MCP request/body limits, per-device concurrency limits, WebSocket size limits, and response-size caps.
- [x] Add TLS/WSS deployment example with Caddy; keep /admin off the public reverse proxy.
- [ ] Add tenant/account scoping to every Manager lookup.
- [ ] Add structured metadata-only audit logs.
- [ ] Run MCP Inspector OAuth flow against a real Auth0/Keycloak/Authentik tenant.

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
