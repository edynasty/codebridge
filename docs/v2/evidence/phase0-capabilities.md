# Phase 0 — capability descriptor evidence

## Assumption

Providers explicitly declare capability availability, never deriving it from harness type or OS; Phase 1 tools declare schemas, permissions/selectors, caller classes, risk, scope, result visibility and idempotency.

## Environment

2026-10-02; frozen provider-contracts §4/§12 and architecture §11/§13. Python JSON Schema validator on this workstation.

## Commands / steps

Created language-neutral `schema/capabilities/v1/provider.schema.json` and `tools.json`. Ran Draft202012 schema validation for provider and all five tool input/output schemas; validated all-false descriptor, then removed `input_monitor` and `resume` separately.

## Observed result

Schemas valid. Missing `input_monitor` and missing `resume` both rejected. Computer flags: `computer.observe`, `computer.input`, `computer.preview`, `input_monitor`, `ax_tree`. Agent flags: `resume`, `history`, `cancel`, `live_events`, `reattach`, `approval_routing`. All flags required, including false flags.

Five tools defined: computer_start/observe/action/status/stop. No Phase 0 Computer tool is registered by these definitions. Actions are a tagged union of eight frozen primitives; frame id mandatory on input; metadata/image planes separated. Local MCP disabled by default; permission and signed peer/provider health checks must all pass before availability. No provider-type/OS fallback. All definition-level schemas include error categories and widget-only `_meta` visibility. Phase 1 implementation must enforce catalog requirements, not assume schema definitions provide runtime policy.

## Conclusion

**PASS — definition milestone only.** Not a Computer Use implementation or engine validation.

## Architecture impact

No frozen decision changes; contracts refined as scheduled for Phase 0. V1 adapters not modified or relabeled as V2 capability providers.

## Follow-up

Phase 1 registry consumes descriptors and validates results; providers advertise flags from actual permission/monitor state. Add runtime availability and policy enforcement only in Phase 1.
