import Foundation

/// Input-free suspension spike for the Computer engine.
///
/// This models the load-bearing Phase 0 question — *can queued input replay after the machine comes
/// back?* — without any input injection, without ScreenCaptureKit and without a GUI session.
///
/// Contract: `docs/v2/architecture.md` §5.5 (sleep / screen lock / screen saver / fast user switch →
/// Computer `suspended`, `controller = none`, frames/geometry invalidated, queued input discarded,
/// fresh observation before resume) and §11.4–§11.5 (controller epoch, frame citation, fail closed).
///
/// This is a state-machine spike: it proves the intended decisions are expressible and testable. It
/// is **not** OS evidence about what macOS does during a lock, and it never posts input events.
public enum ComputerSessionState: String, Codable, Equatable, Sendable {
    case starting
    case active
    case suspended
    case closed
    case failed
}

public enum ComputerControllerKind: String, Codable, Equatable, Sendable {
    case none
    case agent
    case human
}

public enum SuspensionReason: String, Codable, Equatable, Sendable {
    case screenLock
    case screenSaver
    case sleep
    case fastUserSwitch
    case displayChange
    case ipcLoss
    case emergencyStop
    case localInputPreemption
    case permissionRevoked
    case unknown

    /// Human-originated transitions put the session on human hold (architecture §11.4).
    public var isHumanOriginated: Bool {
        switch self {
        case .emergencyStop, .localInputPreemption:
            return true
        default:
            return false
        }
    }
}

public struct FrameStamp: Equatable, Sendable {
    public var frameID: String
    public var arbiterInstance: String
    public var controllerEpoch: Int
    public var geometryGeneration: Int
    public var capturedAt: TimeInterval

    public init(
        frameID: String,
        arbiterInstance: String,
        controllerEpoch: Int,
        geometryGeneration: Int,
        capturedAt: TimeInterval
    ) {
        self.frameID = frameID
        self.arbiterInstance = arbiterInstance
        self.controllerEpoch = controllerEpoch
        self.geometryGeneration = geometryGeneration
        self.capturedAt = capturedAt
    }
}

public enum FrameValidation: Equatable, Sendable {
    case valid
    case unknownFrame
    case arbiterRestarted
    case staleController
    case geometryChanged
    case expired(age: TimeInterval)

    public var rejectionReason: String? {
        switch self {
        case .valid: return nil
        case .unknownFrame: return "stale_frame"
        case .arbiterRestarted: return "stale_frame"
        case .staleController: return "controller_changed"
        case .geometryChanged: return "geometry_changed"
        case .expired: return "stale_frame"
        }
    }
}

public struct QueuedInputAction: Equatable, Sendable {
    public var actionID: String
    public var frameID: String
    public var kind: String

    public init(actionID: String, frameID: String, kind: String) {
        self.actionID = actionID
        self.frameID = frameID
        self.kind = kind
    }
}

public enum QueueAdmission: Equatable, Sendable {
    case queued
    case rejectedNotActive
    case rejectedNoAgentController
    case rejectedFrameInvalid(FrameValidation)
}

public enum ControlRequestOutcome: Equatable, Sendable {
    case granted(epoch: Int)
    case rejected(reason: String)
}

public enum SystemReturnOutcome: Equatable, Sendable {
    case stillSuspendedAwaitingFreshObservation
    case requiresHumanResume
    case cannotReturnWhileClosed
}

public struct ControllerTransition: Equatable, Sendable {
    public var reason: SuspensionReason
    public var previousController: ComputerControllerKind
    public var controller: ComputerControllerKind
    public var controllerEpoch: Int
    public var discardedActions: Int
    public var geometryGeneration: Int
    public var invalidatedFrameID: String?
    public var requiresHumanResume: Bool
    public var state: ComputerSessionState
}

public final class ComputerSuspensionModel {
    public let arbiterInstance: String
    public let maxFrameAge: TimeInterval

    public private(set) var state: ComputerSessionState = .starting
    public private(set) var controller: ComputerControllerKind = .none
    public private(set) var controllerEpoch: Int = 0
    public private(set) var geometryGeneration: Int = 1
    public private(set) var lastFrameID: String?
    public private(set) var queuedActions: [QueuedInputAction] = []
    public private(set) var discardedActionCount: Int = 0
    public private(set) var humanHold = false
    public private(set) var transitions: [ControllerTransition] = []

    private var frames: [String: FrameStamp] = [:]

    public init(arbiterInstance: String = UUID().uuidString, maxFrameAge: TimeInterval = 30) {
        self.arbiterInstance = arbiterInstance
        self.maxFrameAge = maxFrameAge
    }

    // MARK: - Frames

    @discardableResult
    public func recordFrame(_ stamp: FrameStamp) -> FrameValidation {
        guard state != .closed else { return .unknownFrame }
        frames[stamp.frameID] = stamp
        lastFrameID = stamp.frameID
        return validate(frameID: stamp.frameID, now: stamp.capturedAt)
    }

    public func validate(frameID: String, now: TimeInterval) -> FrameValidation {
        guard let stamp = frames[frameID] else { return .unknownFrame }
        guard stamp.arbiterInstance == arbiterInstance else { return .arbiterRestarted }
        guard stamp.controllerEpoch == controllerEpoch else { return .staleController }
        guard stamp.geometryGeneration == geometryGeneration else { return .geometryChanged }
        let age = now - stamp.capturedAt
        guard age <= maxFrameAge else { return .expired(age: age) }
        return .valid
    }

    // MARK: - Lifecycle

    @discardableResult
    public func activate(frame: FrameStamp) -> FrameValidation {
        let validation = recordFrame(frame)
        guard validation == .valid else { return validation }
        state = .active
        controller = .none
        humanHold = false
        return validation
    }

    /// Initial acquisition `none → agent` requires an active session and a valid cited frame.
    public func acquireAgentControl(frameID: String, now: TimeInterval) -> ControlRequestOutcome {
        guard state == .active else {
            return .rejected(reason: "not_active")
        }
        if humanHold {
            return .rejected(reason: "human_hold")
        }
        let validation = validate(frameID: frameID, now: now)
        guard validation == .valid else {
            return .rejected(reason: validation.rejectionReason ?? "invalid_frame")
        }
        controllerEpoch += 1
        controller = .agent
        return .granted(epoch: controllerEpoch)
    }

    public func enqueue(_ action: QueuedInputAction, now: TimeInterval) -> QueueAdmission {
        guard state == .active else { return .rejectedNotActive }
        guard controller == .agent, !humanHold else { return .rejectedNoAgentController }
        let validation = validate(frameID: action.frameID, now: now)
        guard validation == .valid else { return .rejectedFrameInvalid(validation) }
        queuedActions.append(action)
        return .queued
    }

    /// Single serialized drain. Any invalid precondition discards the remainder of the batch.
    public func drainForExecution(frameID: String, now: TimeInterval) -> [QueuedInputAction] {
        guard state == .active, controller == .agent, !humanHold else {
            discardQueue()
            return []
        }
        guard validate(frameID: frameID, now: now) == .valid else {
            discardQueue()
            return []
        }
        let actions = queuedActions
        queuedActions.removeAll()
        return actions
    }

    /// Suspension: `controller = none`, epoch++, frames/geometry invalid, queue discarded.
    @discardableResult
    public func suspend(reason: SuspensionReason) -> ControllerTransition {
        let previous = controller
        let invalidated = lastFrameID
        let discarded = queuedActions.count
        discardedActionCount += discarded
        queuedActions.removeAll()
        frames.removeAll()
        lastFrameID = nil
        controller = .none
        controllerEpoch += 1
        geometryGeneration += 1
        if reason.isHumanOriginated || previous == .human {
            humanHold = true
        }
        if state != .closed {
            state = .suspended
        }
        let transition = ControllerTransition(
            reason: reason,
            previousController: previous,
            controller: controller,
            controllerEpoch: controllerEpoch,
            discardedActions: discarded,
            geometryGeneration: geometryGeneration,
            invalidatedFrameID: invalidated,
            requiresHumanResume: humanHold,
            state: state
        )
        transitions.append(transition)
        return transition
    }

    /// The system came back (unlock, wake, session active). Control is never restored implicitly.
    public func noteSystemReturn() -> SystemReturnOutcome {
        switch state {
        case .closed, .failed:
            return .cannotReturnWhileClosed
        case .suspended:
            return humanHold ? .requiresHumanResume : .stillSuspendedAwaitingFreshObservation
        default:
            return .stillSuspendedAwaitingFreshObservation
        }
    }

    /// Leaving suspension requires a fresh observation; leaving human hold additionally requires a
    /// human actor (never the model path).
    public func resumeAfterFreshObservation(
        frame: FrameStamp,
        humanActor: Bool,
        now: TimeInterval
    ) -> ControlRequestOutcome {
        guard state == .suspended else {
            return .rejected(reason: "not_suspended")
        }
        if humanHold && !humanActor {
            return .rejected(reason: "human_hold")
        }
        let validation = recordFrame(frame)
        guard validation == .valid else {
            return .rejected(reason: validation.rejectionReason ?? "invalid_frame")
        }
        state = .active
        if humanActor {
            humanHold = false
        }
        return .granted(epoch: controllerEpoch)
    }

    public func close() {
        discardQueue()
        controller = .none
        state = .closed
    }

    public var pendingActionCount: Int { queuedActions.count }

    private func discardQueue() {
        discardedActionCount += queuedActions.count
        queuedActions.removeAll()
    }
}
