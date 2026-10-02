import Foundation

/// Where an observed input event came from, as far as this engine can prove.
public enum InputEventOrigin: String, Codable, Equatable, Sendable {
    case engineInjected
    case physicalHardware
    case syntheticOtherProcess
    case unknown
}

public struct InputEventSample: Equatable, Sendable {
    /// `kCGEventSourceUnixProcessID`: 0 for hardware, otherwise the posting process.
    public var sourceProcessID: Int32
    public var eventTypeRawValue: UInt32
    public var secureInputEnabled: Bool

    public init(sourceProcessID: Int32, eventTypeRawValue: UInt32, secureInputEnabled: Bool) {
        self.sourceProcessID = sourceProcessID
        self.eventTypeRawValue = eventTypeRawValue
        self.secureInputEnabled = secureInputEnabled
    }
}

public enum InputPreemptionDecision: Equatable, Sendable {
    case ignoreEngineEvent
    case preempt(reason: String)
}

/// Conservative physical-input classification.
///
/// Rule (architecture §11.6 + Phase 0 rule): only events that are provably posted by **this**
/// engine process are ignored. Everything else — hardware (`pid == 0`) or an unknown synthetic
/// source — preempts agent control. "Unknown" never resolves to "safe".
public enum PhysicalInputClassifier {
    public static func origin(sample: InputEventSample, engineProcessID: Int32) -> InputEventOrigin {
        if sample.sourceProcessID == engineProcessID {
            return .engineInjected
        }
        if sample.sourceProcessID == 0 {
            return .physicalHardware
        }
        if sample.sourceProcessID > 0 {
            return .syntheticOtherProcess
        }
        return .unknown
    }

    public static func decision(origin: InputEventOrigin) -> InputPreemptionDecision {
        switch origin {
        case .engineInjected:
            return .ignoreEngineEvent
        case .physicalHardware:
            return .preempt(reason: "physical_input")
        case .syntheticOtherProcess:
            return .preempt(reason: "unknown_synthetic_input")
        case .unknown:
            return .preempt(reason: "unknown_input_origin")
        }
    }

    public static func decision(sample: InputEventSample, engineProcessID: Int32) -> InputPreemptionDecision {
        decision(origin: origin(sample: sample, engineProcessID: engineProcessID))
    }
}

/// Whether the listen-only event tap is verifiably running.
public enum InputMonitorState: String, Codable, Equatable, Sendable {
    /// No tap: the monitor cannot be installed.
    case unavailable
    /// The tap was created but delivery is unproven (for example `CGPreflightListenEventAccess()`
    /// was false). Conservative: this does **not** count as a usable monitor.
    case installedUnverified
    /// Tap created, preflight granted and no tap-disabled event in the window.
    case verifiedRunning
    /// The tap was created but the system disabled it (`tapDisabledByTimeout` / `byUserInput`).
    case verifiedRunningButDisabled

    public var isUsable: Bool { self == .verifiedRunning }
}

/// `computer.input` is unavailable unless the local-input monitor is verifiably running
/// (architecture §11.6: "If the monitor cannot be installed, `computer.input` is unavailable —
/// fail closed"). A tap whose delivery could not be proven counts as unavailable, not as safe.
public enum InputAvailabilityPolicy {
    public static func inputIsAvailable(monitor: InputMonitorState, secureInputEnabled: Bool) -> Bool {
        monitor.isUsable && !secureInputEnabled
    }

    public static func unavailabilityReason(monitor: InputMonitorState, secureInputEnabled: Bool) -> String? {
        if secureInputEnabled { return "secure_input_active" }
        switch monitor {
        case .verifiedRunning:
            return nil
        case .unavailable:
            return "input_monitor_unavailable"
        case .installedUnverified:
            return "input_monitor_unverified_delivery"
        case .verifiedRunningButDisabled:
            return "input_monitor_disabled"
        }
    }
}
