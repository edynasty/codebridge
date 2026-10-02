# Runtime Development Plan

This plan has priority over full Computer Use implementation.

## Priority summary

| Priority | Goal | Why |
| --- | --- | --- |
| P0 | Reliable asynchronous Runtime foundation | Removes ChatGPT polling and survives disconnects |
| P1 | Harness sessions + readable history | Removes black-box behavior |
| P2 | Manager/Client Runtime UI | Makes runs inspectable without ChatGPT |
| P3 | ChatGPT Runtime widget + completion notification | Gives OMO-style asynchronous user experience |
| P4 | Computer Use model-control MVP | Adds GUI control after lifecycle is reliable |
| P5 | WebRTC live Computer view + takeover | Adds Muse/Spark-style live desktop |
| P6 | Additional platforms/performance | Release hardening |

## P0 — asynchronous Runtime foundation

### RT-001 — Durable ordered RunEvent journal — **NOW**

- [x] Define `RunEvent` with run ID, monotonically increasing sequence, timestamp, source, kind and raw payload.
- [x] Persist events in the existing local SQLite run store.
- [x] Keep the in-memory ring as a cache, not the source of truth.
- [x] Add `EventsAfter(runID, seq, limit)`.
- [x] Persist lifecycle events as well as harness raw events.
- [x] Add migration-compatible tests.

**Acceptance**

- Client restart preserves the event history and sequence.
- A consumer can replay only events missed after a known sequence.
- Existing runs.db files remain usable.

### RT-002 — Run subscriptions

- [x] Add local RunManager subscriptions so terminal/progress changes are pushed rather than polled.
- [x] Bound subscriber queues.
- [x] Slow subscribers may lose live delivery but can recover from the journal.
- [x] Add concurrency/race-oriented tests. (The current Client container has no C compiler, so Go `-race` instrumentation cannot run there; the concurrent subscribe/cancel regression is present and normal tests are green.)

### RT-003 — gRPC unsolicited run events

- [x] Add a `run_event` envelope path independent of an outstanding tool request.
- [x] Client forwards newly journaled events to Manager.
- [x] Manager accepts idempotent ordered events.
- [x] Do not tie run execution to connection lifetime.

### RT-004 — reconnect reconciliation

- [x] On reconnect advertise active/recent run heads.
- [x] Manager requests/reconciles missing sequences.
- [x] Replay missed terminal events.
- [x] Add disconnect/reconnect integration test, including a run that completes while fully offline.

### RT-005 — remove polling as default workflow

- [x] Update agent tool descriptions/skills to state: start once, do not continuously poll.
- [x] Keep `agent_status` for explicit inspection/recovery.
- [x] Add regression coverage that a long run survives the originating request/session and reconciles afterward.

## P1 — Harness sessions and readable history

### RT-006 — HarnessAdapter abstraction

- [x] Define live event + durable session adapter surface.
- [x] Keep harness-native raw records.
- [x] Add opaque session locator; never expose physical home paths upstream.

Validation: the new abstraction/privacy tests pass in an isolated Go compile environment, and `git diff --check` passes in the CodeBridge workspace. The current Client container itself has no Go toolchain, so the full repository suite must still be rerun in the normal build/CI environment.

### RT-007 — OMP adapter

- [x] Parse `--mode json` live events.
- [x] Discover the matching OMP session JSONL.
- [x] Index new JSONL records incrementally.
- [x] Normalize explicit user/assistant/tool records to display entries.
- [x] Verify against multiple real OMP sessions.

Validation: `go test ./...` and `git diff --check` pass with the temporary Go 1.26 toolchain in the Client container. A read-only real-session probe listed/read five sessions under `PI_CODING_AGENT_DIR=/state/omp`, and exact task/cwd discovery resolved the latest 1611-byte task back to its originating OMP session ID.

### RT-008 — Codex adapter

- [x] Parse live Codex output.
- [x] Discover Codex rollout/session JSONL.
- [x] Incrementally index rollout records.
- [x] Normalize explicit conversation/tool records.

Validation: Codex CLI is pinned to 0.159.2 in the Client image, `codex exec` now emits JSONL with `--json --full-auto`, and only host `auth.json` is seeded into persistent `CODEX_HOME=/state/codex`. Synthetic adapter tests, a read-only smoke against the host's real `~/.codex` rollouts, `go test ./...`, `sh -n deploy/client-entrypoint.sh`, and `git diff --check` all pass. Reasoning records remain raw/local and are not projected as conversation entries.

### RT-009 — OpenCode adapter

- [ ] Discover its durable local session source.
- [ ] Implement the same best-effort normalized display projection.

### RT-010 — Session query API

- [ ] Add local query by run/session.
- [ ] Pagination/cursor support.
- [ ] Conversation and raw-event projections.
- [ ] Never invent hidden reasoning.

## P2 — Local and Manager observability

### RT-011 — Client Runs/Sessions UI

- [ ] Add Runs/Sessions section to local UI.
- [ ] Session list + search/filter.
- [ ] Conversation, Activity, Runtime and Raw tabs.
- [ ] Show process/session freshness and run health.

### RT-012 — health telemetry

- [ ] Process alive/exit state.
- [ ] Last live event timestamp.
- [ ] Last durable-session update.
- [ ] Optional safe child-process summary.
- [ ] `possibly_stalled` heuristic with no automatic kill.

### RT-013 — Manager Run Event Broker

- [ ] Track live run heads by account/device.
- [ ] Relay events to authorized browser clients.
- [ ] Keep only bounded metadata/ring buffers server-side.
- [ ] Account-scope every run/session lookup.

### RT-014 — Manager Runs UI

- [ ] Active/history run list.
- [ ] Readable conversation loaded from Client on demand.
- [ ] Activity/runtime views.
- [ ] Offline/stale indicators.

## P3 — ChatGPT asynchronous UX

### RT-015 — SSE Runtime channel

- [ ] Session-scoped authenticated SSE endpoint.
- [ ] Replay from `Last-Event-ID` / sequence.
- [ ] Event filtering by account/run.

### RT-016 — ChatGPT Runtime widget

- [ ] CC Switch-style readable Conversation tab.
- [ ] Activity/Runtime/Raw tabs.
- [ ] Files summary when available.
- [ ] Computer tab when a Computer session exists.
- [ ] Cancel action.

### RT-017 — completion wake-up

- [ ] Widget receives terminal event.
- [ ] Widget sends a follow-up message to ChatGPT.
- [ ] ChatGPT calls `agent_result` once and reports the result.
- [ ] Prevent duplicate wake-ups.

### RT-018 — missed completion inbox

- [ ] Persist small terminal-notification metadata when no widget is connected.
- [ ] Deliver on next authorized widget connection.
- [ ] Mark delivered idempotently.
- [ ] Never store full transcript/output in notification rows.

### RT-019 — asynchronous E2E

- [ ] Start a fake long run.
- [ ] End originating MCP call/connection.
- [ ] Disconnect/reconnect Client.
- [ ] Replay events.
- [ ] Deliver exactly one completion notification.
- [ ] Fetch final result once.

## P4 — Computer Use model control

Continue [computer-use-todo.md](computer-use-todo.md) after RT-001 through RT-019 establish the Runtime foundation.

Minimum first slice:

- Computer session lifecycle;
- macOS capture and logical coordinates;
- input actions;
- MCP observe/action/stop;
- local permissions;
- fake-driver E2E.

## P5 — live Computer view

- WebRTC media path;
- Runtime Widget Computer tab;
- TURN fallback;
- exclusive human/agent controller;
- takeover/resume.

## P6 — release hardening

- latency/bandwidth metrics;
- load limits;
- Windows/Linux backends;
- isolated desktop evaluation.

## Development rule for OMP reuse

RT-001–RT-005 are complete in source and the full Go test suite is green. Do **not** delegate CodeBridge implementation back to OMP yet: first deploy the updated Client/Manager and run a real asynchronous/reconnect smoke with the new Runtime protocol. Only after that smoke passes should OMP be reintroduced for later development tasks.
