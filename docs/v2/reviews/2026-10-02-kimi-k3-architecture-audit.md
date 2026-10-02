# Kimi K3 Architecture Audit — 2026-10-02

Status: **completed; remediation applied; final freeze still pending**

Reviewer: **Kimi K3**  
Review mode: independent architecture challenge after the Opus 5.5 architecture revision  
Scope: current CodeBridge V2 working tree; architecture/security/process boundaries/Computer Use/Runtime/upstream reuse/Phase 0 sufficiency

## 1. Original verdict

The audit returned:

```text
NOT READY_TO_FREEZE
```

It reported **no P0 findings** and enumerated **six P1 findings**:

1. remote-only use had no Phase 1 human approval path;
2. `local_ui` asserted human presence without proving an authenticated human gesture;
3. daemon access to macOS TCC-protected project roots was unspecified;
4. pointer/app-scope enforcement had a remaining injection-time TOCTOU gap;
5. `type` / `keypress` did not have an explicit keyboard-focus scope check;
6. Phase 0 did not cover protected-root TCC identity or lock/screen-saver/fast-user-switch behavior.

> Note: the source verdict text says “Five P1s” but immediately lists six P1 findings (1.1, 2.1, 3.1, 4.1, 4.2, 8.1). This record uses the enumerated count: **6 P1**.

The audit also registered several P2 observations but, by review rule, did not require changes for them.

## 2. P1 disposition

### K1 — Remote-only approval gap

**Finding:** Phase 1 allowed only local UI approvals while the product also targets remote Secure MCP Tunnel use. An unmatched `ask` could block a remote-started ComputerSession until the user physically returned to the Mac.

**Decision:** **Accepted.**

**Remediation:**

- move a minimal `remote_human` approval-only path into Phase 1;
- no WebRTC dependency;
- opt-in only;
- scopes limited to `once` / `session`;
- never `project` / `always`;
- never `secret.read` / `cloud.upload`;
- approval token delivered only through tool-result `_meta`;
- model-visible paths still cannot approve.

Affected documents:

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

### K2 — `local_ui` human-presence assumption

**Finding:** signed-app IPC identity alone does not prove that a human actually performed the approval.

**Decision:** **Accepted with explicit residual-risk boundary.**

**Remediation:**

- `local_ui` now requires an explicit foreground approval gesture in CodeBridge.app;
- v1 does **not** claim LAContext / Touch ID proof for every approval;
- the trust boundary is explicitly “verified CodeBridge.app + explicit UI gesture”;
- compromised app process remains a documented residual risk;
- stronger authenticated-presence checks may later be added for selected high-impact grants without changing Policy.

Affected documents:

- `architecture.md` / `architecture.zh-CN.md`

### K3 — Daemon protected-root TCC

**Finding:** a project under `~/Documents`, `~/Desktop`, `~/Downloads` or another TCC-protected location could fail with opaque host errors if `codebridged` lacks its own stable Files-and-Folders authorization.

**Decision:** **Accepted.**

**Remediation:**

- Phase 0 now includes a protected-root TCC spike for the per-user LaunchAgent;
- it must test authorization persistence across rebuild/update;
- Project registration must probe the root;
- missing host/TCC access maps to:
  `permission_denied: host_permission_required`;
- if a stable daemon authorization is not viable, protected roots are explicitly unsupported in v1 instead of failing later.

Affected documents:

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`
- `provider-contracts.md` / `provider-contracts.zh-CN.md`

### K4 — Pointer/app-scope injection-time race

**Finding:** checking the frontmost app and target point before a batch was not enough; a window or protected surface can change before CGEvent delivery.

**Decision:** **Accepted.**

**Remediation:**

- checks 1–5 are re-evaluated immediately before every low-level event;
- pointer events re-hit-test the current window/application under the point;
- app scope and protected-surface checks are per-event;
- an observable scope/focus/protected-surface change aborts the batch;
- the remaining OS-level race between final query and CGEvent delivery is documented as a residual limitation rather than claimed away.

Affected documents:

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

### K5 — Keyboard focus scope

**Finding:** `type` / `keypress` have no coordinate, so pointer-style point scoping does not protect them from focus changes.

**Decision:** **Accepted.**

**Remediation:**

- every emitted key event requires a fresh focused/frontmost-app check;
- focus must remain inside the granted app scope and outside protected targets;
- focus change to an out-of-scope or protected target aborts the remainder.

Affected documents:

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

### K6 — Missing Phase 0 closure for lock/TCC behavior

**Finding:** two load-bearing assumptions lacked explicit spikes:
1. stable daemon Files-and-Folders TCC identity;
2. ScreenCaptureKit/CGEvent behavior during screen lock, screen saver, wake and fast user switching.

**Decision:** **Accepted.**

**Remediation:**

Phase 0 now explicitly tests both.

Computer safety rule during these transitions:

```text
ComputerSession -> suspended
controller -> none
invalidate frames/geometry
discard queued input
fresh observation before resume
```

No queued agent input may replay after return.

Affected documents:

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

## 3. P2 observations intentionally not promoted

The audit registered but did not block on:

- synthetic input from other Accessibility-permitted processes;
- provider-specific orphan process reattach behavior;
- future Run.kind expansion;
- Session archival policy;
- possible Windows bias in the CodexBridge `win` branch;
- widget WebRTC fallback details.

These remain non-blocking until their documented trigger is reached.

## 4. Post-remediation state

All six P1 findings have corresponding V2 document changes.

This audit does **not** itself grant `READY_TO_FREEZE`. The next step is an independent final Freeze Gate against the remediated working tree.

Expected gate behavior:

- report only remaining P0/P1;
- do not redesign for style/preference;
- if none remain, return `READY_TO_FREEZE`.

## 5. Repository status at audit closure

No implementation code was changed as part of this audit remediation.

The architecture review changes remain uncommitted until the final Freeze Gate passes.
