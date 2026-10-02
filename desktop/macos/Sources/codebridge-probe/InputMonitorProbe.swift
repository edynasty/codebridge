import Carbon.HIToolbox
import CodeBridgeComputerSpike
import CoreGraphics
import Foundation

/// Observation counters for a listen-only tap. Event *content* is never recorded: only counts by
/// conservative origin classification and by preemption decision.
final class TapObservationCounter {
    private(set) var originCounts: [String: Int] = [:]
    private(set) var decisionCounts: [String: Int] = [:]
    private(set) var disabledEvents = 0
    var secureInputObserved = false
    let engineProcessID: Int32

    init(engineProcessID: Int32) {
        self.engineProcessID = engineProcessID
    }

    func recordDisabled() {
        disabledEvents += 1
    }

    func record(type: CGEventType, event: CGEvent) {
        let sourceProcessID = Int32(truncatingIfNeeded: event.getIntegerValueField(.eventSourceUnixProcessID))
        let sample = InputEventSample(
            sourceProcessID: sourceProcessID,
            eventTypeRawValue: type.rawValue,
            secureInputEnabled: IsSecureEventInputEnabled()
        )
        let origin = PhysicalInputClassifier.origin(sample: sample, engineProcessID: engineProcessID)
        secureInputObserved = secureInputObserved || sample.secureInputEnabled
        originCounts[origin.rawValue, default: 0] += 1

        let decision = PhysicalInputClassifier.decision(origin: origin)
        switch decision {
        case .ignoreEngineEvent:
            decisionCounts["ignore_engine_event", default: 0] += 1
        case .preempt(let reason):
            decisionCounts["preempt:\(reason)", default: 0] += 1
        }
    }
}

/// Input Monitoring probe.
///
/// Creates a **listen-only** session event tap (never a modifying or posting tap), observes for the
/// requested window, and reports whether the monitor is verifiably running. Input is reported
/// available only in that case (`docs/v2/architecture.md` §11.6).
enum InputMonitorProbe {
    static func run(duration: TimeInterval) -> InputMonitorSection {
        let preflight = InputMonitoringPreflightReport(
            preflight_listen_access: CGPreflightListenEventAccess(),
            preflight_post_access: CGPreflightPostEventAccess(),
            secure_input_enabled: IsSecureEventInputEnabled()
        )

        let counter = TapObservationCounter(engineProcessID: getpid())
        let mask = eventMask()
        let callback: CGEventTapCallBack = { _, type, event, userInfo in
            guard let userInfo else { return Unmanaged.passUnretained(event) }
            let counter = Unmanaged<TapObservationCounter>.fromOpaque(userInfo).takeUnretainedValue()
            if type == .tapDisabledByTimeout || type == .tapDisabledByUserInput {
                counter.recordDisabled()
                return Unmanaged.passUnretained(event)
            }
            counter.record(type: type, event: event)
            return Unmanaged.passUnretained(event)
        }

        guard let tap = CGEvent.tapCreate(
            tap: .cgSessionEventTap,
            place: .headInsertEventTap,
            options: .listenOnly,
            eventsOfInterest: mask,
            callback: callback,
            userInfo: Unmanaged.passUnretained(counter).toOpaque()
        ) else {
            return InputMonitorSection(
                preflight: preflight,
                tap_created: false,
                tap_error: "CGEvent.tapCreate returned nil: the Input Monitoring monitor could not be installed "
                    + "for this process, or there is no GUI session",
                tap_disabled_count: 0,
                observation_seconds: 0,
                event_counts_by_origin: [:],
                preemption_decisions: [:],
                monitor_state: InputMonitorState.unavailable.rawValue,
                input_available: false,
                unavailability_reason: InputAvailabilityPolicy.unavailabilityReason(
                    monitor: .unavailable, secureInputEnabled: preflight.secure_input_enabled
                ),
                injected_events: 0,
                hardware_events_observed: 0,
                delivery_proof: "not_installed",
                note: "listen-only tap; no event was posted, modified or stored"
            )
        }

        let source: CFRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0)
        CFRunLoopAddSource(CFRunLoopGetCurrent(), source, .commonModes)
        CGEvent.tapEnable(tap: tap, enable: true)
        let deadline = Date().addingTimeInterval(max(0, duration))
        counter.secureInputObserved = preflight.secure_input_enabled
        while Date() < deadline {
            RunLoop.current.run(mode: .default, before: min(deadline, Date().addingTimeInterval(0.1)))
            counter.secureInputObserved = counter.secureInputObserved || IsSecureEventInputEnabled()
        }
        CGEvent.tapEnable(tap: tap, enable: false)
        CFRunLoopRemoveSource(CFRunLoopGetCurrent(), source, .commonModes)
        CFMachPortInvalidate(tap)
        counter.secureInputObserved = counter.secureInputObserved || IsSecureEventInputEnabled()

        let hardwareObserved = counter.originCounts[InputEventOrigin.physicalHardware.rawValue] ?? 0
        let state: InputMonitorState
        let deliveryProof: String
        if counter.disabledEvents > 0 {
            state = .verifiedRunningButDisabled
            deliveryProof = "tap_disabled"
        } else if !preflight.preflight_listen_access || !CGPreflightListenEventAccess() {
            // Tap creation succeeded while the preflight said no: delivery is unproven, so this is
            // deliberately NOT treated as a usable monitor.
            state = .installedUnverified
            deliveryProof = "preflight_false"
        } else if hardwareObserved == 0 {
            // No hardware event arrived in the window; delivery cannot be proven either way.
            state = .installedUnverified
            deliveryProof = "none_in_window"
        } else {
            state = .verifiedRunning
            deliveryProof = "hardware_event_observed"
        }

        return InputMonitorSection(
            preflight: preflight,
            tap_created: true,
            tap_error: nil,
            tap_disabled_count: counter.disabledEvents,
            observation_seconds: max(0, duration),
            event_counts_by_origin: counter.originCounts,
            preemption_decisions: counter.decisionCounts,
            monitor_state: state.rawValue,
            input_available: InputAvailabilityPolicy.inputIsAvailable(
                monitor: state, secureInputEnabled: counter.secureInputObserved
            ),
            unavailability_reason: InputAvailabilityPolicy.unavailabilityReason(
                monitor: state, secureInputEnabled: counter.secureInputObserved
            ),
            injected_events: 0,
            hardware_events_observed: hardwareObserved,
            delivery_proof: deliveryProof,
            note: "listen-only tap; input is reported available only when preflight allows it, the tap is "
                + "installed, it was never disabled, at least one hardware event was observed, and Secure Input "
                + "was not observed active. Synthetic events "
                + "from other processes and events with unknown origin are classified as preempting "
                + "(conservative: unknown never resolves to safe)."
        )
    }

    private static func eventMask() -> CGEventMask {
        let types: [CGEventType] = [
            .leftMouseDown, .leftMouseUp, .rightMouseDown, .rightMouseUp,
            .otherMouseDown, .otherMouseUp,
            .leftMouseDragged, .rightMouseDragged, .otherMouseDragged,
            .scrollWheel, .keyDown, .keyUp, .flagsChanged,
        ]
        var mask: CGEventMask = 0
        for type in types {
            mask |= CGEventMask(1) << CGEventMask(type.rawValue)
        }
        return mask
    }
}
