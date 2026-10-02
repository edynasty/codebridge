# CodeBridge Computer Use Architecture

Status: **design baseline / implementation target**

Computer Use depends on the asynchronous [Runtime architecture](runtime.md). Runtime lifecycle, event replay, session UI and completion notification are implemented first; Computer Use contributes model observation/input and the WebRTC Computer tab rather than creating a separate lifecycle system.

This document defines Computer Use for CodeBridge: ChatGPT or another vision-capable agent can observe and control a local computer, while the user can watch the session live inside the ChatGPT plugin UI and take control when needed.

The design extends the existing CodeBridge trust model instead of replacing it.

## 1. Goals

Computer Use must provide:

- local desktop observation as model-readable screenshots;
- pointer, keyboard, scroll, drag and wait actions;
- a long-lived computer session with explicit ownership and cancellation;
- a live human preview embedded in the ChatGPT UI;
- optional human takeover without racing the agent for input;
- low model/network overhead: live video must not be sent through MCP tool results;
- local permission gates and audit metadata consistent with existing `bash` / `agent` controls;
- a transport design that works behind NAT and degrades cleanly when peer-to-peer media is unavailable;
- macOS first, with a backend abstraction for Windows/Linux later.

Non-goals for the first release:

- replacing the existing gRPC device transport;
- sending 15–30 FPS screenshots to the model;
- arbitrary remote `pyautogui`/script execution as the Computer Use API;
- audio streaming;
- multi-controller simultaneous input on one display;
- a public unattended remote-desktop product.

## 2. Current CodeBridge baseline

The current implementation already has the right control-plane shape:

```text
ChatGPT / MCP client
        |
        | HTTPS / MCP Streamable HTTP
        v
+---------------------------+
| CodeBridge Manager        |
| MCP tools + account scope |
| device registry / routing |
+-------------+-------------+
              |
              | gRPC bidirectional stream
              | AgentService.Connect
              v
+---------------------------+
| CodeBridge Client         |
| local policy + execution  |
| workspaces / agent runs   |
+---------------------------+
```

The gRPC stream is long-lived, authenticated with the per-device credential, carries request/response/progress envelopes, and already has keepalive, reconnect and account-scoped routing semantics.

Computer Use must reuse that control plane.

> The device control plane is the existing gRPC `AgentService.Connect` bidirectional stream. Historical WebSocket references may remain only in migration history or completed regression records.

## 3. Core design decision

Computer Use is split into **control/AI observation** and **human live media**.

```text
                              ChatGPT
                    +------------+-------------+
                    |                          |
               Model/tool path            UI widget
                    |                          |
                  MCP                      WebRTC
                    |                    live preview
                    v                          |
             CodeBridge Manager               |
                    |                          |
             gRPC control plane               |
                    |                          |
                    v                          |
             CodeBridge Client <--------------+
                    |
             Computer Runtime
                    |
              Local Desktop
```

### 3.1 Control plane — existing gRPC

gRPC remains authoritative for:

- device registration and heartbeat;
- session create/close/cancel;
- model-originated computer actions;
- model observation requests;
- permission requests and grants;
- action status/errors;
- capability advertisement;
- audit/event metadata.

The Computer Use implementation must not introduce a second command authority for the model.

### 3.2 AI observation plane — still images

The model sees **explicit screenshots**, not a video stream.

Typical loop:

```text
computer_observe
      |
      v
Client captures a high-quality still image
      |
      v
gRPC response
      |
      v
MCP image content
      |
      v
ChatGPT vision
      |
      v
computer_action(actions[])
```

Observation is requested:

- explicitly by `computer_observe`; or
- automatically after an action batch when `observe_after=true`.

The client may suppress redundant screenshots using a perceptual/change hash, but the model-facing API must remain deterministic: if the caller explicitly requests an observation, the response describes whether a new frame or the latest unchanged frame was returned.

### 3.3 Human media plane — WebRTC

The live preview shown to the user is separate from the model.

```text
macOS ScreenCaptureKit
        |
        | encoded video
        v
CodeBridge Computer Runtime
        |
        | WebRTC video track
        +-----------------------------> ChatGPT Computer Widget
        |
        +-- ICE/STUN direct path when possible
        +-- TURN relay fallback when required
```

The Manager participates in signaling and authorization, not normal media forwarding.

Benefits:

- video frames do not consume MCP/model context;
- OpenAI model request volume is independent from preview FPS;
- P2P media avoids routing high-bandwidth video through the Manager when NAT allows it;
- TURN remains an explicit, measurable fallback.

A low-FPS WebSocket/JPEG preview may exist only as a development/fallback mode. It is not the primary production media plane.

## 4. Session model

A Computer Use session is a first-class resource.

```text
ComputerSession
  id
  account_id
  device_id
  display_id
  state
  controller
  created_at
  expires_at / lease
  capabilities
  preview_state
```

Suggested states:

```text
starting -> active -> stopping -> closed
              |
              +-> paused
              +-> failed
```

Controller values:

- `agent` — ChatGPT/model actions are accepted;
- `human` — widget/user input is accepted and agent input is paused;
- `none` — observation only.

### 4.1 Exclusive controller

Only one controller may inject input into a display/session at a time.

This prevents:

- ChatGPT and the user typing simultaneously;
- two agent runs fighting over one cursor;
- stale actions being applied after a takeover.

Takeover transition:

```text
agent
  |
  | user presses Take over
  v
input barrier / pending action drain
  |
  v
human

human
  |
  | Resume agent
  v
fresh observation required
  |
  v
agent
```

After human control returns to the agent, the next agent action must be preceded by a fresh observation.

## 5. MCP tool surface

Keep the public surface small.

### `computer_start`

Creates a session.

Input, conceptually:

```json
{
  "device_id": "mbp-m1",
  "display_id": "main",
  "mode": "interactive",
  "preview": true
}
```

Returns:

- `session_id`;
- display logical dimensions;
- pixel dimensions / scale factor;
- capabilities;
- initial observation when requested;
- preview/widget bootstrap metadata when available.

### `computer_observe`

Returns one model-readable screenshot and screen metadata.

The screenshot must be emitted as native MCP image content, not base64 embedded into a text JSON field.

### `computer_action`

Executes one ordered batch.

Supported V1 actions:

- `click`;
- `double_click`;
- `move`;
- `drag`;
- `scroll`;
- `type`;
- `keypress`;
- `wait`.

Example:

```json
{
  "session_id": "cmp_...",
  "actions": [
    {"type":"click","x":640,"y":420,"button":"left"},
    {"type":"type","text":"hello"},
    {"type":"keypress","keys":["ENTER"]}
  ],
  "observe_after": true
}
```

The batch executes in order and stops on the first failed action.

### `computer_stop`

Stops input/capture resources, invalidates preview credentials and releases the display lease.

## 6. Coordinates and displays

Model coordinates use **logical pixels**.

For a Retina display:

```json
{
  "width": 1728,
  "height": 1117,
  "pixel_width": 3456,
  "pixel_height": 2234,
  "scale_factor": 2
}
```

The model receives an observation normalized to logical dimensions. Therefore the point it sees at `(x,y)` is the point it clicks at `(x,y)`.

The platform backend owns logical-to-physical conversion.

Multi-display V1 rules:

- the session binds to one display;
- switching display requires an explicit session operation or a new session;
- screenshots and input coordinates never silently cross display coordinate spaces.

## 7. Local Computer Runtime

Add a platform-neutral package with OS backends.

Conceptual interface:

```go
type Driver interface {
    Capabilities(context.Context) (Capabilities, error)
    Displays(context.Context) ([]Display, error)
    Capture(context.Context, CaptureRequest) (Frame, error)
    Execute(context.Context, []Action) error
    StartPreview(context.Context, PreviewConfig) (PreviewHandle, error)
    Close(context.Context) error
}
```

Recommended package shape:

```text
internal/computer/
  service.go
  session.go
  actions.go
  frame.go
  permissions.go
  driver.go
  driver_darwin.go
  driver_windows.go      # later
  driver_linux.go        # later
```

For macOS, the preferred production backend is:

- ScreenCaptureKit for capture/live preview;
- CoreGraphics/CGEvent for pointer and keyboard injection;
- explicit Screen Recording + Accessibility permission checks.

If the Go implementation becomes awkward or fragile around ScreenCaptureKit, use a small signed Swift helper behind a local Unix-domain socket/stdio protocol. The public CodeBridge protocol must not depend on whether the macOS backend is in-process or helper-based.

## 8. Permission model

Computer Use extends the existing local permission system.

Recommended permission verbs:

- `computer.observe`;
- `computer.input`;
- `computer.preview`.

Rules remain local. A remote caller cannot enable Computer Use for itself.

V1 behavior:

- starting an observation-only session requires `computer.observe`;
- any pointer/keyboard action requires `computer.input`;
- live preview requires `computer.preview`;
- deny overrides allow;
- an unmatched rule produces an approval request;
- session-level `once` approval may authorize subsequent actions in that session, bounded by its lease.

High-consequence confirmations remain distinct from transport permission. A local `computer.input=allow` means the agent may operate the desktop; it does not mean every destructive business action is implicitly approved.

## 9. Live ChatGPT widget

The CodeBridge plugin UI should render a Computer Session card.

Minimum UI:

```text
+------------------------------------------------+
| MacBook Pro · Computer session        LIVE     |
|                                                |
|               desktop preview                  |
|                                                |
| Agent control · Chrome                         |
+------------------------------------------------+
| Pause | Take over | Resume agent | Stop        |
+------------------------------------------------+
```

Widget responsibilities:

- display the WebRTC video track;
- show session/device/state/current controller;
- show agent cursor/action overlay when available;
- request/release human takeover;
- stop the session;
- adapt preview quality to visibility/size;
- never expose the long-lived device credential.

The widget receives only a short-lived, session-scoped preview token.

## 10. Signaling and media authorization

The Manager owns signaling endpoints because it already owns account/device/session authorization.

Suggested flow:

1. MCP creates `ComputerSession`.
2. Manager creates a short-lived preview token scoped to `account + session + view/control role`.
3. Widget exchanges SDP/ICE through authenticated signaling endpoints.
4. Client receives signaling over the existing control channel or a dedicated signaling endpoint.
5. WebRTC establishes direct media when possible.
6. TURN is used only when direct ICE fails.

Do not reuse:

- OAuth access tokens as WebRTC credentials;
- device credentials in the browser;
- permanent TURN credentials.

TURN credentials should be ephemeral.

## 11. Bandwidth and latency budget

Live preview and model observation have independent budgets.

Recommended defaults:

| State | Human preview | Model observation |
| --- | --- | --- |
| widget hidden/background | 360p, 2–5 FPS | none |
| inline | 720p, 10–15 FPS | on demand |
| fullscreen | up to 1080p, 20–30 FPS | on demand |
| agent action loop | independent | still image after meaningful state changes |

Additional controls:

- adaptive bitrate;
- pause video when the widget is not visible;
- cap TURN bitrate separately;
- perceptual/change hash before automatic model observations;
- coalesce rapid input actions into one batch;
- UI-settle delay before automatic post-action observation.

The high-bandwidth path is browser/WebRTC traffic, not MCP model traffic.

## 12. Failure behavior

The system must fail closed for input.

Examples:

- gRPC disconnect: stop accepting model actions; preview may be marked stale and then closed;
- WebRTC disconnect: model control can continue if explicitly allowed, but widget shows preview unavailable;
- widget closes: preview track may stop after a grace period; the ComputerSession does not necessarily end;
- client permission revoked: immediately reject input/capture and terminate affected sessions;
- human takeover loses connectivity: controller returns to `none`, not automatically to `agent`;
- display resolution changes: invalidate coordinates and force a fresh observation.

## 13. Security and privacy invariants

- No live video is written to Manager persistence.
- No screenshots are persisted by default.
- Device credentials never reach the widget.
- Preview tokens are short-lived, scoped and revocable.
- Computer input is local-permission gated.
- Every session/action/takeover/stop emits metadata-only audit events.
- Account scoping applies to every ComputerSession lookup.
- Session IDs alone never authorize access.
- Model screenshots are bounded in dimensions and bytes.
- Media bitrate, session count and action rate are bounded.
- Secure/password fields may be visible to the local capture stack; the product must warn users that Computer Use can observe anything visible on the selected display.

## 14. Capability advertisement

Device registration should eventually advertise explicit computer capabilities, for example:

```json
{
  "computer": {
    "available": true,
    "platform": "darwin",
    "observe": true,
    "input": true,
    "preview": true,
    "displays": 1
  }
}
```

Do not infer availability only from the OS. Permissions may make capture or input unavailable at runtime.

## 15. Rollout

### Phase A — model-control MVP

- gRPC protocol/session contract;
- macOS capture;
- macOS input;
- observe/action/stop MCP tools;
- permissions and tests.

No live video required.

### Phase B — live preview

- WebRTC signaling;
- WebRTC video track;
- ChatGPT widget;
- adaptive preview;
- TURN fallback.

### Phase C — takeover and hardening

- human input channel;
- controller barrier;
- audit/limits;
- reconnect/failure semantics;
- latency/bandwidth instrumentation.

### Phase D — additional platforms

- Windows backend;
- Linux backend;
- optional isolated/virtual desktop environments.

## 16. Architectural rule

The defining rule for this feature is:

> **MCP/gRPC carries intent, control and model observations. WebRTC carries human-facing live media.**

Do not turn the gRPC stream into a video transport, and do not turn the MCP/model channel into a frame stream.
