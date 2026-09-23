# Development Todo

This file tracks engineering execution and quality work. Product capability milestones remain in [../TODO.md](../TODO.md).

Todo IDs are stable references for implementation, commits, reviews, and test evidence.

## P0 — release and security gates

### INT-001 Real ChatGPT OAuth linking

- [ ] Choose a currently RFC 8707-compatible external authorization server for the production smoke.
- [ ] Run `codebridge-doctor --url ...` and pass authorization-server metadata checks.
- [ ] Obtain a real user access token for the exact CodeBridge resource.
- [ ] Pass authenticated Doctor `list_devices`.
- [ ] Pass authenticated Doctor `project_info` through a live Agent.
- [ ] Link the same MCP endpoint in ChatGPT.
- [ ] Verify first authenticated `list_devices` Tool call from ChatGPT.
- [ ] Verify one safe source-reading flow from ChatGPT.
- [ ] Verify reconnect/reauthorization or refresh behavior.
- [ ] Confirm audit records contain subject/tool/device/workspace metadata but no source body/token/path leakage.
- [ ] Record provider/version/date and exact verified configuration in the OAuth provider guide.

**Done when:** real ChatGPT UI → OAuth provider → CodeBridge `/mcp` → Manager → live Client succeeds and the evidence/configuration is documented.

### MGR-001 Tenant/account scoping

- [ ] Define `account_id`/tenant ownership model.
- [ ] Bind OAuth subject/account to device visibility.
- [ ] Add tenant/account key to persistent device records.
- [ ] Scope Registry lookups by account.
- [ ] Scope MCP `list_devices`, `list_workspaces`, and every device Tool call by account.
- [ ] Scope Admin operations appropriately.
- [ ] Add cross-tenant negative tests for every lookup path.
- [ ] Migrate existing single-user state safely.
- [ ] Update architecture, OAuth, admin, and deployment docs.

**Done when:** an authenticated subject cannot discover or call another account's device even when it knows the device ID.

### SEC-001 Filesystem security regression matrix

- [ ] Add table-driven cases for absolute path, `..`, symlink escape, sensitive symlink alias, broken symlink, and sensitive workspace root.
- [ ] Add filename edge cases for spaces, Unicode, quoted Git paths, and rename output.
- [ ] Add nested sensitive directory cases.
- [ ] Add tests for path replacement/race resistance around direct reads.
- [ ] Add Windows-specific path cases where behavior differs.
- [ ] Verify every file-returning Tool uses the same policy boundary.

**Done when:** all current file/discovery/search/git surfaces share a tested containment + sensitivity policy.

### SEC-002 Secret leakage regression suite

- [ ] Seed test workspaces with recognizable fake secrets.
- [ ] Assert fake secrets never appear in default `read_file`, discovery, search, git status, or git diff output.
- [ ] Assert physical workspace roots never appear in success or error responses.
- [ ] Assert audit JSONL never contains fake source content, tokens, enrollment codes, credentials, search queries, or physical paths.
- [ ] Assert local sensitive-file opt-in restores only the intentionally documented behavior.

**Done when:** one integration suite proves the default remote surface cannot expose seeded secret material.

### CI-001 Race detector gate

- [ ] Add `make test-race`.
- [ ] Run `go test -race ./...` in Linux CI.
- [ ] Fix or explicitly document any package that cannot run under race detection.
- [ ] Ensure Registry pending-call/disconnect/reconnect tests run under the race detector.

**Done when:** race CI is required and green.

### CI-002 Static/security analysis

- [ ] Add `govulncheck ./...` CI.
- [ ] Add `staticcheck ./...` CI.
- [ ] Add a repository secret scan.
- [ ] Pin tool versions in CI.
- [ ] Document how developers run the same checks locally.

**Done when:** new dependency vulnerabilities, common Go static-analysis defects, and committed secrets fail CI.

### CI-003 Unified local CI command

- [ ] Add `make ci`.
- [ ] Include fmt check, tests, vet, build, and other mandatory local-safe gates.
- [ ] Keep CI workflow commands aligned with `make ci`.
- [ ] Document the command in README and development standards.

**Done when:** one local command reproduces the main non-cross-platform CI gate.

### OPS-001 Production deployment smoke

- [ ] Add a non-secret deployment smoke script around `codebridge-doctor`.
- [ ] Check HTTPS health.
- [ ] Check RFC 9728 metadata.
- [ ] Check authorization-server metadata.
- [ ] Check public Admin blocking.
- [ ] Optionally accept an environment-provided real access token.
- [ ] Optionally verify one device/workspace via metadata-only `project_info`.
- [ ] Return machine-readable non-zero status on failure.

**Done when:** a deployment can be acceptance-tested without manually assembling curl commands.

### OPS-002 Release artifact smoke

- [ ] Verify release checksums in CI.
- [ ] Start each native Linux release binary in a smoke job where practical.
- [ ] Verify `--version` output on packaged Manager/Client/Doctor.
- [ ] Verify release archive contents do not contain credentials, local state, or source-test fixtures with fake secrets.
- [ ] Document rollback to the previous release.

**Done when:** a tag proves the published artifacts, not only source builds, are usable.

## P1 — coding intelligence

### MCP-101 `find_symbol`

- [ ] Define language-neutral input/output schema.
- [ ] Return logical paths only.
- [ ] Bound result count and runtime.
- [ ] Apply sensitive-file policy.
- [ ] Add Java/TypeScript/Go fixtures.
- [ ] Add MCP integration test.

### MCP-102 `find_references`

- [ ] Define stable symbol identity/input.
- [ ] Prefer LSP when available.
- [ ] Add bounded fallback strategy.
- [ ] Apply sensitive-file policy.
- [ ] Add false-positive/false-negative fixtures.

### MCP-103 `read_symbol`

- [ ] Return the smallest useful symbol range.
- [ ] Bound bytes/lines.
- [ ] Preserve logical path only.
- [ ] Apply sensitive-file policy.
- [ ] Add truncation behavior/tests.

### AGT-101 LSP adapter foundation

- [ ] Define local adapter interface.
- [ ] Implement process lifecycle/cancellation.
- [ ] Never expose arbitrary LSP process control remotely.
- [ ] Add Java adapter.
- [ ] Add TypeScript adapter.
- [ ] Add Go adapter.
- [ ] Add startup timeout and crash recovery.
- [ ] Keep all indexes/processes on the Client machine.

### AGT-102 tree-sitter fallback

- [ ] Define supported languages.
- [ ] Keep parsing/indexing local.
- [ ] Bound repository scan size/time.
- [ ] Apply sensitive-file exclusions before parsing.
- [ ] Add equivalence tests against LSP for representative fixtures.

### AGT-103 local code index

- [ ] Define cache directory and lifecycle.
- [ ] Never upload index to Manager.
- [ ] Add size limit/eviction.
- [ ] Exclude sensitive paths.
- [ ] Invalidate safely on repository changes.
- [ ] Document how to delete/rebuild the index.

### MCP-104 repository dependency graph

- [ ] Define graph schema.
- [ ] Support Maven first.
- [ ] Add npm/Go support later.
- [ ] Bound graph size.
- [ ] Keep physical paths out of graph output.
- [ ] Add representative multi-module fixtures.

## P2 — controlled write mode

Do not start P2 until P0 security gates and account scoping are complete.

### SEC-201 Per-workspace write opt-in

- [ ] Add explicit local configuration.
- [ ] Default disabled.
- [ ] Remote MCP caller cannot enable it.
- [ ] Advertise write capability only for opted-in workspaces.
- [ ] Audit every write attempt.

### MCP-201 `apply_patch` only

- [ ] Define patch schema.
- [ ] Reject arbitrary shell/command execution.
- [ ] Restrict writes to opted-in workspace.
- [ ] Apply the same containment/symlink/sensitive policy.
- [ ] Bound patch size/files changed.
- [ ] Reject binary writes initially.

### AGT-201 Git checkpoint

- [ ] Verify repository state before write.
- [ ] Create recoverable local checkpoint.
- [ ] Refuse unsafe/ambiguous dirty-state cases unless policy explicitly permits them.
- [ ] Never push automatically.

### MCP-202 Diff preview and confirmation

- [ ] Generate proposed diff before mutation when possible.
- [ ] Require explicit confirmation metadata for modifying Tool call.
- [ ] Revalidate file state between preview and apply.
- [ ] Return final logical-path diff summary.

### AGT-202 Rollback

- [ ] Define rollback identifier.
- [ ] Keep rollback local.
- [ ] Test rollback after partial failure.
- [ ] Bound retained checkpoints.

## Engineering maintenance

### DX-001 Pull request template

- [ ] Add Todo ID field.
- [ ] Add security-boundary impact field.
- [ ] Add tests run field.
- [ ] Add documentation/config sync field.
- [ ] Add Definition-of-Done checklist.

### DX-002 Issue templates

- [ ] Security regression template.
- [ ] Feature template with acceptance criteria.
- [ ] External integration/compatibility template including provider/version/date.

### DX-003 Branch protection baseline

- [ ] Require CI before merge.
- [ ] Prevent force-push to shared protected `main`.
- [ ] Require up-to-date branch for merge when multiple contributors are active.
- [ ] Document maintainer emergency procedure.

### DX-004 Dependency update policy

- [ ] Define update cadence.
- [ ] Require release-note/security review for MCP SDK, OAuth/JWT, WebSocket, and crypto-related dependencies.
- [ ] Run full integration suite after critical dependency upgrades.
- [ ] Record externally visible compatibility changes in docs.

### DX-005 Architecture decision records

- [ ] Add lightweight `docs/adr/` format.
- [ ] Record decisions that change trust boundaries, persistence, authentication, Tool mutability, or multi-tenancy.
- [ ] Link superseding ADRs rather than silently rewriting historical rationale.

## Per-task execution checklist

Copy this into the working Todo for every non-trivial change:

```text
<ID> <task name>
- [ ] acceptance criteria written
- [ ] architecture/security impact reviewed
- [ ] implementation complete
- [ ] unit/regression tests complete
- [ ] relevant integration test complete
- [ ] limits/cancellation reviewed
- [ ] make fmt-check
- [ ] go test ./...
- [ ] go vet ./...
- [ ] make build
- [ ] race test when concurrency/security relevant
- [ ] docs/config examples synchronized
- [ ] final CI green
- [ ] Todo state synchronized
- [ ] fixup commits squashed
```
