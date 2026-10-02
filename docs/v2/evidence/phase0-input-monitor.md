# Phase 0 — Input monitoring and physical-input preemption feasibility

Status: **PARTIAL — listen-only harness implemented and measured; input is reported UNAVAILABLE (conservative), real preemption UNTESTED (needs a signed, granted build)**

Owner: NativeSpike (`desktop/macos`, native evidence)
Frozen contract: [Architecture](../architecture.md) §11.5–§11.6, §13.6; [Provider Contracts](../provider-contracts.md) §6; [Roadmap](../roadmap.md) Phase 0 (lock-state spike, Phase 1 "Local physical-input preemption").

## 1. What was built

| Path | Purpose |
| --- | --- |
| `desktop/macos/Sources/codebridge-probe/InputMonitorProbe.swift` | creates a **listen-only** `.cgSessionEventTap`, observes for a bounded window, counts events by origin, never posts or modifies an event |
| `desktop/macos/Sources/codebridge-probe/InputMonitorProbe.swift` (`TapObservationCounter`) | per-event classification: `kCGEventSourceUnixProcessID` only — never key codes, never text |
| `desktop/macos/Sources/CodeBridgeComputerSpike/InputClassifier.swift` | `PhysicalInputClassifier` + `InputAvailabilityPolicy` |
| `desktop/macos/Sources/codebridge-probe/ProbeRunner.swift` | emits the `input_monitor` section and the fail-closed `blocked` finding |

Classification rule implemented (architecture §11.6, conservative):

```text
source pid == engine pid  -> engineInjected      -> ignore
source pid == 0           -> physicalHardware    -> preempt (physical_input)
source pid >  0 (other)   -> syntheticOtherProcess -> preempt (unknown_synthetic_input)
anything else / unknown   -> unknown             -> preempt (unknown_input_origin)
```

"No user input detected" is therefore never a *safe* conclusion: an event that cannot be attributed to the
engine preempts, and an engine that cannot verify its monitor reports input unavailable.

Availability rule implemented:

```text
tap not created                          -> monitor_state = unavailable                  -> input available = false
tap created, no preflight, no hardware   -> monitor_state = installedUnverified          -> input available = false
tap created, preflight ok, no hardware   -> monitor_state = installedUnverified          -> input available = false
tap created, preflight ok, hardware seen -> monitor_state = verifiedRunning              -> input available = true
tap created but disabled by the system   -> monitor_state = verifiedRunningButDisabled    -> input available = false
```

## 2. Observed on this host (2026-10-02)

Same ad-hoc probe binary (`Signature=adhoc`, `TeamIdentifier=not set`), two launch contexts:

| Measurement | shell child (`ppid` = bun) | `launchctl submit` job (`ppid` = 1), repeated twice | child of a development-mode `codebridged` |
| --- | --- | --- | --- |
| `CGPreflightListenEventAccess()` | **true** | **false** (both runs) | **true** |
| `CGPreflightPostEventAccess()` | **true** | **false** (both runs) | **true** |
| `CGEvent.tapCreate(.cgSessionEventTap, .listenOnly)` | **created** | **created** (both runs) | **created** |
| `tapDisabledByTimeout` / `disabledByUserInput` | 0 | 0 | 0 |
| hardware events observed in a 5 s window | 0 | 0 | 0 |
| `IsSecureEventInputEnabled()` | false (true in one separate run) | false | false |
| `monitor_state` | `installedUnverified` (`none_in_window`) | `installedUnverified` (`preflight_false`) | `installedUnverified` (`none_in_window`) |
| `input_available` | **false** (`input_monitor_unverified_delivery`) | **false** (`input_monitor_unverified_delivery`) | **false** (`input_monitor_unverified_delivery`) |
| events injected | 0 | 0 | 0 |

```bash
P=desktop/macos/.build/release/codebridge-probe
$P input-monitor --json --pretty --duration 5 --output /tmp/cb-im-shell.json
launchctl submit -l com.codebridge.phase0.p3 -- "$P" input-monitor --json --duration 5 --output /tmp/cb-im-launchd.json
# daemon child (frozen host.phase0_probe path; daemon started with CODEBRIDGE_PHASE0_DEBUG=1 + CODEBRIDGE_PHASE0_PROBE)
bin/codebridged ctl probe --host-socket /tmp/cb-native-phase0/run/hostipc.sock --probe input-monitor
```

### 2.1 What this establishes (observed)

1. **`CGPreflightListenEventAccess()` and tap creation disagree on this host.** In the launchd-owned context the
   preflight returned `false` while a listen-only session tap was created successfully — reproducibly. Preflight
   alone is therefore not a usable gate, and tap creation alone is not proof that events are delivered.
2. Because no hardware input occurred in any measured window, delivery is unproven, and the implemented policy
   reports `input_monitor_unverified_delivery` and **`computer.input` unavailable** — exactly the fail-closed
   requirement ("report input unavailable until evidence").
3. Preflight answers again differ by launch context (`true` as a shell/app child, `false` under launchd); see
   `phase0-tcc.md` §2. This is consistent with context-dependent permission attribution; the mechanism is not
   proven here.
4. The tap was never disabled by the system in any window (`disabled = 0`) and no event was posted
   (`injected_events = 0`), so the harness itself produced no input.

## 3. Untested — exact prerequisites

| Question | Status | Prerequisite |
| --- | --- | --- |
| Physical hardware input preempts the agent immediately | **UNTESTED** | signed, granted build (Input Monitoring) + a real arbiter; a human must type/move the mouse while the engine holds control. Phase 0 must not inject input, so the preempting event cannot be synthesised here |
| Synthetic input from **another** Accessibility-permitted process | **UNTESTED** | a second signed, AX-permitted process; classification logic is covered by unit tests (conservative → preempt) but not by a live observation |
| Whether a tap that reports `preflight=false` still delivers events | **UNTESTED** | requires an observed hardware event in the launchd context |
| Behaviour under secure input (`IsSecureEventInputEnabled() == true` observed once) | **UNTESTED** | a controlled window with a password prompt/secure field; the harness records the flag but no input was available to classify |
| Behaviour during lock / screen saver / fast user switch | **UNTESTED** | see `phase0-lock-state.md` |
| Held-input release on abort/preemption (Phase 1) | **not implemented in Phase 0** | Phase 1 single serialized injector |

Until at least one hardware event is observed and classified in a granted build, `computer.input` must remain
unavailable (fail closed) — this is the honest Phase 0 answer, not a limitation of the harness.

## 4. Commands for the parent

```bash
P=desktop/macos/.build/release/codebridge-probe

# baseline (idle window): expect 0 events, input unavailable
$P input-monitor --json --pretty --duration 5 --output /tmp/cb-im-idle.json

# with a human deliberately typing/moving during the window (still no injection by the probe):
$P input-monitor --json --pretty --duration 15 --output /tmp/cb-im-human.json
#   -> hardware_events_observed > 0 and monitor_state == verifiedRunning would be the first evidence that
#      the monitor is usable; anything else stays "unavailable"

# launch-context comparison
launchctl submit -l com.codebridge.phase0.p3 -- "$P" input-monitor --json --duration 5 --output /tmp/cb-im-launchd.json
launchctl remove com.codebridge.phase0.p3
```

Safety: the probe never requests Input Monitoring, never posts or modifies events, never stores key content,
never locks/sleeps the machine and never touches the TCC database.

## 5. Parent Secure Input negative path

The actual daemon-child window observed `IsSecureEventInputEnabled() == true`, zero injected events, and unavailable input with `secure_input_active`. Availability now requires both a verified monitor **and Secure Input disabled**; the probe tracks Secure Input at start, end and event delivery, and rechecks Listen preflight at the end. A behavioral regression verifies that Secure Input blocks even an otherwise verified monitor. Swift suite: 33 tests PASS.

Hardware/third-party/own-injection discrimination and reliable physical preemption remain unverified. `computer.input` stays unavailable; neither creation of a tap nor a zero-event idle window is sufficient proof. [Measured result](phase0-parent-smoke.json). No additional input or permission experiments after F2.

