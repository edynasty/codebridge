# Phase 0 — Lock / sleep / session-switch behaviour (user Task 7)

Status: **PARTIAL — harness implemented and observed on this host; real transition behaviour UNTESTED (no transition occurred in the measured windows, and no signed/granted build exists for the ScreenCaptureKit+CGEvent half)**

Owner: NativeSpike (`desktop/macos`, native evidence)
Frozen contract: [Architecture](../architecture.md) §5.5 (sleep / screen lock / screen saver / fast user switch row), §11.4 (controller state machine), §11.5 (frame validity); [Roadmap](../roadmap.md) Phase 0 "Lock-state behavior".

## 1. What was built

| Path | Purpose |
| --- | --- |
| `desktop/macos/Sources/codebridge-probe/LockStateProbe.swift` | observes (never triggers) `NSWorkspace` sleep/wake/screens/session notifications and `com.apple.screenIsLocked` / `com.apple.screenIsUnlocked`; snapshots `CGSessionCopyCurrentDictionary()` |
| `desktop/macos/Sources/CodeBridgeComputerSpike/SuspensionModel.swift` | input-free suspension state machine: `suspend(reason:)` → `controller = none`, epoch++, frames + geometry invalidated, queued input discarded; `noteSystemReturn()`; `resumeAfterFreshObservation` |
| `desktop/macos/Sources/CodeBridgeComputerSpike/InputClassifier.swift` | conservative preemption classification (see `phase0-input-monitor.md`) |
| `desktop/macos/Tests/CodeBridgeComputerSpikeTests/SuspensionSpikeTests.swift` | 9 tests, including "queued input cannot replay after suspension" |

The probe reads the session dictionary and installs observers only. It does **not** call `pmset`, does **not**
lock or sleep the machine, does **not** create power assertions, and does **not** inject input.

## 2. Observed on this host (2026-10-02)

```bash
$ desktop/macos/.build/release/codebridge-probe lock-state --json --pretty --duration 5
```

```json
{
  "initial": {
    "on_console": true,
    "screen_locked": null,
    "session_dictionary_keys": ["kCGSSessionLoginwindowSafeLogin", "kCGSSessionOnConsoleKey", "kCGSessionLoginDoneKey"]
  },
  "final": { "on_console": true, "screen_locked": null },
  "events": [],
  "os_transitions_observed": [],
  "observation_seconds": 5,
  "model_transitions_applied": [],
  "queue_replay_blocked": true,
  "evidence_class": "os_observation:none_in_window + state_machine_spike"
}
```

- the machine was unlocked and on-console throughout; **no** lock / sleep / screen-saver / fast-user-switch
  transition occurred in the window, so nothing about macOS transition behaviour is claimed from this run;
- `screen_locked` is `null` because the key `CGSSessionScreenIsLocked` was absent from the session dictionary
  while unlocked (only `kCGSSessionOnConsoleKey` / loginwindow keys were present);
- the state-machine spike (below) is reported in the same JSON but is labelled separately, because it is **not**
  OS evidence.

## 3. State-machine spike (proven by unit tests, NOT OS evidence)

`ComputerSuspensionModel` implements the frozen reaction and is covered by tests that pass:

```text
Executed 26 tests, with 0 failures   (desktop/macos: swift test)
  SuspensionModelTests (9): queue discard + no replay, epoch/geometry invalidation, human hold,
                            stale-frame expiry, arbiter restart, acquisition-epoch rule
```

Rules encoded (each with an assertion):

| Rule (architecture §5.5/§11.4/§11.5) | Test |
| --- | --- |
| suspension sets `controller = none` and increments `controller_epoch` | `testQueuedInputIsDiscardedOnSuspensionAndCannotReplay` |
| suspension discards the queued actions (`discardedActions == 1`, queue empty afterwards) | same |
| after the system returns, the session is still `suspended`; a pre-suspension frame cannot execute or drain | same |
| frames are invalidated on suspension (epoch and geometry move) | `testResumeRequiresAFreshObservationFrame`, `testGeometryChangeInvalidatesFramesPlannedAgainstTheOldGeometry` |
| resuming requires a *fresh* observation frame; the old frame is rejected | `testResumeRequiresAFreshObservationFrame` |
| a human-originated hold (`emergencyStop`, `localInputPreemption`) cannot be left by the model path | `testHumanHoldRequiresAHumanActorToResume` |
| frame age bound rejects stale frames | `testStaleFrameIsRejectedAfterTheAgeBound` |
| controller acquisition itself bumps the epoch, so the acquisition frame cannot be reused | `testAcquisitionBumpsTheEpochSoTheAcquisitionFrameCannotBeReused` |

**This is a state-machine spike, not OS evidence.** It shows the required decisions are expressible and
testable, e.g. "queued input cannot replay after unlock" is proven **inside the model**, not proven about the
window server's CGEvent queues.

## 4. Untested — exact prerequisites

| Question from the roadmap | Status | Prerequisite |
| --- | --- | --- |
| ScreenCaptureKit behaviour across lock / screen saver / wake / fast user switch | **BLOCKED** | signed, granted build (Screen Recording) to capture before/after; currently `preflight=false`, real call `denied -3801` in every context measured |
| CGEvent behaviour across lock / wake (are queued events flushed? does injection fail closed?) | **BLOCKED, not measured** | required Phase 0 OS evidence is missing; no injection was attempted, and F2 now stops dependent Computer experiments. A model-only queue discard does not close this acceptance criterion. |
| Real transition observation on this host | **UNTESTED so far** | run the harness and perform the transition manually (see §5); no transition happened during the 5 s window measured above |
| Fast user switching | **UNTESTED** | second GUI login session; cannot be created safely from here |

No document may cite §3 as evidence about macOS. The only OS-level facts recorded above are: the session
dictionary keys present while unlocked/on-console, and the absence of transitions in the window.

## 5. How to observe a real transition safely

The harness only listens; a human performs the transition (the probe never locks or sleeps the machine):

```bash
# terminal 1: observe for 120s
desktop/macos/.build/release/codebridge-probe lock-state --json --pretty --duration 120 --output /tmp/cb-lock.json
# then, by hand (any of): lock the screen (⌃⌘Q), start the screen saver, close the lid / sleep, log in as another user
# afterwards:
python3 -c "import json;d=json.load(open('/tmp/cb-lock.json'));print(d['lock_state']['os_transitions_observed'], d['lock_state']['model_transitions_applied'], d['lock_state']['queue_replay_blocked'])"
```

Expected honest outcomes: `os_transitions_observed` lists the notification kinds the OS actually delivered;
`model_transitions_applied` shows what the state machine would do; `queue_replay_blocked` is the spike result.
A field where the OS delivered nothing must be reported as nothing — an absent transition is not a pass.

Fast-user-switch and screen-saver specifics must be measured from a signed build before the §5.5 failure
matrix can be called verified for Phase 1. The daemon-side half of this spike is recorded in `phase0-ipc.md`
(daemon evidence); this file is the native side.

## 6. Parent correction and locked-snapshot measurement

The original model queue smoke seeded a pre-acquisition frame that the epoch check rejected. Its old `queue_replay_blocked=true` field was therefore **not evidence of discarding an admitted action**. The harness now seeds a current-epoch action, suspends on initially/finally locked, off-console or unknown snapshots, and reports replay blocked only after an admitted action was actually discarded by suspension. Screen-saver start/stop uses its own notifications, not display-sleep aliases; callbacks run on the main queue.

An actual daemon child observed a **locked** session at startup: agent → none, controller epoch 2, geometry generation 2, state suspended, **one admitted model action discarded**. No OS transition occurred during that window. [Raw sanitized result](phase0-parent-smoke.json). This proves the snapshot-driven model path, **not** an OS input queue flush, lock/unlock transition, ScreenCaptureKit failure matrix, or CGEvent behavior. Swift behavioral suite passed 33 tests after correction. Tasks requiring real transition and signed OS evidence remain blocked; no input was injected. Further Computer experiments stopped at the F2 finding.

