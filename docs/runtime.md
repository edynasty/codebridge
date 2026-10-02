# CodeBridge Runtime Architecture

Status: **implementation baseline**

CodeBridge Runtime is the common execution layer for long-running local agents and future Computer Use. It exists to make agent work asynchronous, recoverable, observable and presentable without requiring ChatGPT to poll a run continuously.

## 1. Problem statement

The current long-run path already starts agent work outside the lifetime of one MCP request, but the user experience still depends on repeated `agent_status` / `agent_result` calls.

That causes four problems:

- a ChatGPT turn may remain active for too long and accumulate many tool calls;
- transient Manager/Client disconnects make the remote state appear stale even while the local harness keeps running;
- the Manager and Client cannot present a useful live session view beyond `event_count` and one raw `last_event`;
- raw harness output is treated as ephemeral progress even though OMP and Codex both maintain durable local session histories.

Runtime fixes this by separating **execution**, **durable session data**, **transport events**, **presentation**, and **completion notification**.

## 2. Design principles

1. **The local Client owns execution.** A started run must not depend on the MCP request or ChatGPT turn that created it.
2. **No polling as the normal ChatGPT workflow.** `agent_status` remains for manual inspection and recovery only.
3. **Local durable state is authoritative for recovery.** CodeBridge keeps its own run journal and can also index harness-native sessions.
4. **Harness-native history is preserved.** OMP/Codex/OpenCode are adapted, not rewritten into a CodeBridge-specific agent protocol.
5. **Readable conversation is first-class.** The UI should render session history in a CC Switch-style conversation view, not only process telemetry.
6. **Raw events stay available.** Normalized UI entries are a projection; raw harness events remain inspectable locally.
7. **Manager is a relay/control plane.** It should not become a database for source code, full transcripts, screenshots or video.
8. **Computer Use reuses Runtime.** Agent lifecycle, notification, session UI and health are shared with Computer Use rather than duplicated.

## 3. Target architecture

```text
                           ChatGPT
                    +---------+----------+
                    |                    |
                  Model              Runtime Widget
                    |                    |
                   MCP                  SSE
                    |                    |
                    v                    v
                 CodeBridge Manager
                    |
             gRPC bidirectional stream
                    |
                    v
                 CodeBridge Client
          +---------+-----------+
          |                     |
      RunManager           Runtime Event Bus
          |                     |
   Harness Adapter         Event Journal
    /     |      \               |
  OMP   Codex   OpenCode         |
   |      |        |             |
 live   live     live            |
 JSON   events   events          |
   |      |        |             |
 durable harness sessions        |
   \______|________/             |
          |                      |
          +----------+-----------+
                     v
                Session View
                     |
              Computer Runtime
              (when requested)
                     |
                   WebRTC
                     |
               Runtime Widget
```

## 4. Run lifecycle

The existing CodeBridge lifecycle remains:

```text
queued -> running -> completed
                  -> failed
                  -> timeout
                  -> cancelled
                  -> interrupted
```

The important change is that state transitions become durable ordered events.

A run has:

- CodeBridge `run_id`;
- harness type (`omp`, `codex`, `opencode`);
- harness-native session identifier when discovered;
- process identity/health metadata;
- monotonically increasing Runtime event sequence;
- durable local event cursor;
- latest terminal result;
- optional Computer Use session IDs.

## 5. Runtime event journal

Every run produces ordered Runtime events.

Minimum envelope:

```json
{
  "run_id": "run_xxx",
  "seq": 42,
  "at": "2026-09-30T00:00:00Z",
  "source": "omp",
  "kind": "harness.raw",
  "payload": {}
}
```

Core event kinds:

- `run.queued`
- `run.started`
- `run.completed`
- `run.failed`
- `run.cancelled`
- `run.timeout`
- `run.interrupted`
- `harness.raw`
- `harness.session.discovered`
- `conversation.message`
- `tool.started`
- `tool.completed`
- `process.health`
- `computer.started`
- `computer.stopped`

The first implementation may persist raw harness lines plus lifecycle events before richer normalization is available.

### Why the journal exists

The current in-memory ring is useful for short progress display but is insufficient for reconnect replay, Manager resynchronization, UI refresh, historical conversation views and reliable completion notification.

The journal therefore uses a durable cursor/sequence. Consumers request events after the last sequence they have seen.

## 6. Harness adapters

Each harness has an adapter with two data sources when available:

```text
             Harness Adapter
             /             \
       live event source   durable session source
```

### OMP

Primary sources:

- `omp -p --mode json` stdout for low-latency live events;
- OMP session JSONL under the local OMP session store for durable conversation/history.

The adapter should discover the harness-native session without exposing its physical path upstream.

### Codex

Primary sources:

- `codex exec` live output/events;
- Codex rollout/session JSONL under the local Codex session store.

### OpenCode

Use its live structured output and durable session source where available.

### Adapter rule

Do not assume all harnesses expose identical fields. Normalize only stable presentation concepts and retain the raw event.

## 7. Session and conversation model

The UI needs a normalized display model, not a fake unified harness protocol.

Suggested display entries:

```text
message
tool_call
tool_result
file_change
command
test
warning
error
runtime
```

A message entry contains role/text when explicitly present in the harness session. Unknown/new harness records render as `runtime` or remain available under Raw. The product must not invent hidden reasoning that the harness did not expose.

## 8. Runtime UI

Manager UI, Client UI and ChatGPT use the same Runtime event model.

Recommended tabs:

- **Conversation** — CC Switch-style readable session history;
- **Activity** — normalized Runtime event timeline;
- **Files** — changed-file summary when known;
- **Computer** — live WebRTC desktop when Computer Use is active;
- **Runtime** — PID/elapsed/health/connectivity/session metadata;
- **Raw** — harness-native events for debugging.

The local Client may show the richest view because it can read local session stores directly. The Manager shows live relayed events and bounded metadata. Full durable history should be fetched from the connected Client on demand instead of permanently copied into Manager storage.

The ChatGPT widget shows live run/session state and receives events through SSE. It should not require the model to poll.

## 9. Completion notification

Normal flow:

```text
agent_start
   |
   +--> immediate run_id
   |
ChatGPT turn ends

local harness continues
   |
terminal Runtime event
   |
Client -> gRPC -> Manager -> SSE -> Runtime Widget
   |
widget sends follow-up message
   |
ChatGPT calls agent_result once
   |
user receives final report
```

If the widget is not active, the Manager retains a small undelivered completion notification containing only run/session metadata. The next authorized widget connection can deliver it. Do not persist the full agent transcript in the notification store.

## 10. Reconnect and reconciliation

Transient network loss must not lose run state.

On Client reconnect:

1. Client registers normally.
2. Client advertises active and recently terminal runs with their latest Runtime sequence.
3. Manager compares its last received sequence.
4. Missing events are replayed from the local journal.
5. Terminal completion notification is emitted if it was missed.

A network disconnect therefore means temporary loss of observation, not loss of the local run.

## 11. Run health

Useful signals:

- process alive;
- elapsed time;
- last live event time;
- last durable-session update;
- event count;
- child process activity when safely available;
- gRPC connected/disconnected;
- harness terminal status.

The UI may mark a run `possibly_stalled` after a configurable inactivity threshold, but must not automatically kill it solely because output is quiet.

## 12. Computer Use integration

Computer Use is a Runtime capability.

```text
Runtime Session
├── Agent
├── Conversation
├── Activity
├── Files
├── Runtime health
└── Computer
    ├── model observations over MCP/gRPC
    └── human live preview over WebRTC
```

The live desktop is not used to "show OMP". OMP/Codex remain headless. The Computer tab appears when the run actually controls a GUI.

See [computer-use.md](computer-use.md).

## 13. Storage boundaries

### Client may persist

- CodeBridge run metadata;
- ordered Runtime event journal;
- harness session locator/opaque identifier;
- local cursors;
- terminal result;
- local UI history/index.

### Manager may persist

- bounded run metadata;
- last received sequence;
- terminal/completion-notification metadata;
- audit metadata.

### Manager must not persist by default

- source files;
- complete harness transcripts;
- Computer screenshots/video;
- raw keystrokes or typed secrets.

## 14. Public agent tool semantics

- `agent_start`: start and return immediately; normal entry point for long work.
- `agent_status`: manual inspection/recovery only; normal ChatGPT behavior must not continuously poll it.
- `agent_result`: call once after a terminal notification, or manually.
- `agent_runs`: discovery/recovery/history.
- `agent_cancel`: explicit cancellation.

Later Runtime APIs may add `agent_events` with `after_seq`, `agent_session` and `agent_health`.

## 15. Implementation sequence

Runtime is implemented before full Computer Use:

1. durable ordered Runtime events;
2. reconnect reconciliation;
3. harness session adapters;
4. readable Client/Manager session UI;
5. ChatGPT Runtime widget + completion wake-up;
6. Computer Use model-control path;
7. Computer Use WebRTC live view and takeover.

Until the asynchronous Runtime foundation is verified, CodeBridge development should not delegate implementation back into OMP long runs; direct development avoids recursively depending on the subsystem being repaired.
