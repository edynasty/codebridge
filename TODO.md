# Roadmap

## P0 - make the MVP production-safe enough for personal use

- [ ] Add OAuth 2.1 for MCP user authentication.
- [ ] Replace shared enrollment token with one-time enrollment and per-device credentials.
- [ ] Persist account/device metadata only; never persist source payloads.
- [ ] Add device revoke/rotate commands.
- [ ] Add tenant/account scoping to every Manager lookup.
- [ ] Add request/response byte and concurrency limits at the Manager.
- [ ] Add structured metadata-only audit logs.
- [ ] Add TLS/WSS deployment example with Caddy or Nginx.
- [ ] Run official MCP Inspector cases against the real SDK build.

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
