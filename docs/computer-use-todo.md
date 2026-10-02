# Computer Use Development TODO

This is the execution backlog for [computer-use.md](computer-use.md).

**Dependency:** complete the Runtime foundation in [runtime-todo.md](runtime-todo.md) first. Computer Use reuses Runtime run/session lifecycle, event delivery, UI and notification infrastructure.

Rules:

- Execute in order unless an item explicitly says it can run in parallel.
- Every implementation item includes tests and documentation/config synchronization.
- Keep the current gRPC device control plane.
- Do not send live preview frames through MCP/model tool results.
- macOS is the first supported desktop platform.
- A task is complete only when its acceptance checks pass.

## P0 — Baseline and protocol contract

### CU-001 — Synchronize transport documentation

- [x] Update stale README/architecture references from WSS/WebSocket device transport to current gRPC bidi transport.
- [x] Preserve historical wording only where it is explicitly identified as migration history.
- [x] Verify examples use the current Manager gRPC port/config naming.
- [x] Run documentation/link sanity checks available in the repo.

**Acceptance**

- README and `docs/architecture.md` match the actual `AgentService.Connect` implementation.
- No active architecture diagram claims that the current device transport is WSS.

### CU-002 — Define Computer Use protocol types

- [ ] Add platform-neutral session/action/frame/capability types.
- [ ] Define stable action validation: click, double_click, move, drag, scroll, type, keypress, wait.
- [ ] Define logical-pixel display metadata.
- [ ] Define bounded screenshot/frame metadata.
- [ ] Add unit tests for validation and serialization.

**Acceptance**

- Invalid coordinates/action payloads fail before reaching a platform driver.
- Types contain no macOS-specific public contract.

### CU-003 — Advertise device computer capabilities

- [ ] Extend device registration/protocol without breaking old clients.
- [ ] Surface observe/input/preview availability separately.
- [ ] Store capability data only in the online registry; do not persist desktop state.
- [ ] Add compatibility tests for clients that omit the field.

**Acceptance**

- Old clients still register.
- New clients expose runtime capability/permission status.

## P1 — Computer runtime and session lifecycle

### CU-004 — Add ComputerSession manager

- [ ] Add session IDs, state machine, lease/expiry and account/device/display binding.
- [ ] Enforce one input controller per display/session.
- [ ] Add stop/cancel and cleanup on client/device disconnect.
- [ ] Add metadata-only audit events.
- [ ] Add concurrency/expiry tests.

**Acceptance**

- A session cannot be looked up from another account.
- Stale sessions cannot inject input.
- Session cleanup does not leak goroutines/resources.

### CU-005 — Add local Driver abstraction

- [ ] Create `internal/computer` service/driver interfaces.
- [ ] Separate capture, action execution and preview capabilities.
- [ ] Provide a deterministic fake driver for tests.
- [ ] Integrate driver lifetime with Client runtime reload/shutdown.

**Acceptance**

- Manager/client integration tests can run without controlling the real desktop.
- Runtime reload closes old driver resources safely.

## P2 — macOS model-control MVP

### CU-006 — macOS permission detection

- [ ] Detect Screen Recording permission.
- [ ] Detect Accessibility/input permission.
- [ ] Report observe/input capabilities independently.
- [ ] Add local operator guidance without auto-changing macOS security settings.

**Acceptance**

- Missing permission returns an actionable, non-secret error.
- Capture and input permission states are never conflated.

### CU-007 — macOS screenshot capture

- [ ] Implement selected-display capture.
- [ ] Normalize model-facing screenshot to logical coordinates.
- [ ] Enforce dimension/byte bounds.
- [ ] Add frame IDs/change hashes.
- [ ] Test Retina scale conversion.

**Acceptance**

- A pixel location visible to the model maps to the same logical input coordinate.
- No screenshot is persisted by default.

### CU-008 — macOS pointer/keyboard actions

- [ ] Implement move/click/double-click/drag.
- [ ] Implement scroll.
- [ ] Implement text input and keypress combinations.
- [ ] Implement wait.
- [ ] Stop an action batch on first failure.
- [ ] Add local/fake-driver regression tests.

**Acceptance**

- Ordered action batches are deterministic.
- Input is rejected when input permission is unavailable.

## P3 — CodeBridge control path

### CU-009 — Add computer permission verbs

- [ ] Extend local permission rules for `computer.observe`, `computer.input`, `computer.preview`.
- [ ] Preserve deny-overrides-allow behavior.
- [ ] Support once/session approval without granting permanent broad access accidentally.
- [ ] Add permission tests and UI/config support.

**Acceptance**

- Remote callers cannot enable Computer Use permissions.
- An unmatched input rule never injects input.

### CU-010 — Add gRPC Computer Use request routing

- [ ] Route session/observe/action/stop messages through the existing gRPC device session.
- [ ] Keep existing generic tools compatible.
- [ ] Bound image payloads and action batch size.
- [ ] Add reconnect/cancel tests.

**Acceptance**

- No second persistent command transport is introduced.
- gRPC disconnect fails input closed.

### CU-011 — Add MCP computer tools

- [ ] Add `computer_start`.
- [ ] Add `computer_observe`.
- [ ] Add `computer_action`.
- [ ] Add `computer_stop`.
- [ ] Return screenshots as native MCP image content.
- [ ] Add tool schemas/output validation/account scoping/tool-policy integration.

**Acceptance**

- ChatGPT can observe → act → observe using only the public tools.
- Image bytes are not embedded as base64 text in `structuredContent`.

### CU-012 — End-to-end model-control test

- [ ] Add a fake desktop driver.
- [ ] Exercise MCP → Manager → gRPC → Client → fake desktop.
- [ ] Verify screenshot return, action ordering, permission gating, cancellation and account scoping.
- [ ] Cover old-client compatibility.

**Acceptance**

- CI proves the complete model-control path without requiring macOS GUI access.

## P4 — Human live preview

### CU-013 — Add preview token/signaling model

- [ ] Add short-lived session-scoped preview tokens.
- [ ] Add signaling authorization bound to account/session/device.
- [ ] Define ICE/STUN/TURN configuration without exposing device credentials.
- [ ] Add expiry/revocation tests.

**Acceptance**

- A session ID by itself cannot open a preview.
- Widget never receives the per-device credential.

### CU-014 — macOS WebRTC video publisher

- [ ] Capture via the production macOS capture backend.
- [ ] Publish an encoded video track.
- [ ] Add adaptive resolution/FPS/bitrate controls.
- [ ] Stop/reduce capture when there are no viewers.
- [ ] Add metrics for direct vs TURN path and bitrate.

**Acceptance**

- Normal media bytes do not pass through MCP tool results.
- Preview survives ordinary control-plane action traffic without blocking it.

### CU-015 — ChatGPT Computer Session widget

- [ ] Add a plugin UI resource for the live computer session.
- [ ] Render live preview, session state, device and controller.
- [ ] Add Pause/Take over/Resume/Stop controls.
- [ ] Add inline/fullscreen/PiP behavior where the host supports it.
- [ ] Declare only the required network/CSP domains.
- [ ] Handle preview unavailable without breaking model control.

**Acceptance**

- User can watch the desktop from the conversation UI.
- Closing/failing the preview does not corrupt the computer session.

### CU-016 — TURN fallback and deployment docs

- [ ] Add optional TURN configuration.
- [ ] Use ephemeral TURN credentials.
- [ ] Document NAT/firewall requirements.
- [ ] Add a diagnostic that reports direct/TURN/no-media path without logging media content.

**Acceptance**

- A client behind restrictive NAT has a documented relay path.
- Operators can cap relay bitrate/cost.

## P5 — Human takeover and safety

### CU-017 — Exclusive controller/takeover barrier

- [ ] Implement agent → human takeover.
- [ ] Drain/cancel pending agent input before human control is granted.
- [ ] Implement human → agent resume.
- [ ] Force fresh observation before resumed agent input.
- [ ] Handle takeover connection loss by moving to `none`, not automatic agent control.

**Acceptance**

- Human and agent can never inject input concurrently.

### CU-018 — Human input data channel

- [ ] Add pointer/keyboard input from the widget.
- [ ] Authenticate every data-channel session to the ComputerSession role.
- [ ] Rate-limit input.
- [ ] Reuse the same local input permission gate.

**Acceptance**

- Takeover works without routing each mouse event through the ChatGPT model.

### CU-019 — Limits, audit and privacy hardening

- [ ] Session count limits.
- [ ] Action-rate and batch-size limits.
- [ ] Screenshot size/frequency limits.
- [ ] Preview bitrate limits.
- [ ] Metadata-only audit events for session/action/takeover/stop.
- [ ] Confirm screenshots/video are never persisted by default.

**Acceptance**

- Load/abuse tests cannot make unbounded queues or memory growth.
- Audit logs contain no screenshot/video/input text payloads.

## P6 — Performance and release

### CU-020 — Latency/bandwidth instrumentation

- [ ] Measure observe RTT, action RTT, capture time and model-loop wait separately.
- [ ] Measure preview encoder bitrate/FPS and direct vs TURN path.
- [ ] Add a local diagnostics view.
- [ ] Define default quality profiles: background, inline, fullscreen.

**Acceptance**

- A user can identify whether delay comes from local capture, network, Manager, model loop or TURN.

### CU-021 — macOS release gate

- [ ] Real macOS smoke: observe/click/type/scroll/drag.
- [ ] Retina and at least one external-display scenario.
- [ ] Permission-denied scenarios.
- [ ] Network disconnect/reconnect.
- [ ] Widget preview + takeover smoke.
- [ ] Update README, client docs, deployment docs and security model.

**Acceptance**

- Computer Use is explicitly marked experimental or stable with known limitations.
- All automated tests and the documented real-device smoke pass.

## P7 — Later platforms

### CU-022 — Windows backend

- [ ] Capture.
- [ ] Input.
- [ ] permissions/session integration.
- [ ] live preview.

### CU-023 — Linux backend

- [ ] Wayland/X11 capture strategy.
- [ ] Input strategy.
- [ ] permissions/session integration.
- [ ] live preview.

### CU-024 — Optional isolated desktops

- [ ] Evaluate virtual display/VM/containerized browser desktop.
- [ ] Keep isolated sessions behind the same Driver/ComputerSession contract.
- [ ] Do not block the macOS physical-desktop release on this item.
