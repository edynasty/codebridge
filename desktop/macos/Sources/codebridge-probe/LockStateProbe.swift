import AppKit
import CodeBridgeComputerSpike
import CoreGraphics
import Foundation

/// Ordered recorder for lock/sleep/session notifications.
final class LockEventRecorder {
    private(set) var events: [LockStateEvent] = []

    func record(_ kind: String, source: String, detail: String? = nil) {
        events.append(
            LockStateEvent(at: ProbeReportWriter.timestamp(), kind: kind, source: source, detail: detail)
        )
    }
}

/// Lock-state probe: observes (never triggers) screen lock, screen saver, sleep and fast user switch.
///
/// It also feeds every observed transition into the suspension model spike so the report can show
/// the intended decisions. The report labels OS observation and state-machine application
/// separately: the model output is **not** OS evidence about queued input.
enum LockStateProbe {
    static func run(duration: TimeInterval) -> LockStateSection {
        let model = ComputerSuspensionModel(arbiterInstance: "probe-" + UUID().uuidString)
        let now = Date().timeIntervalSince1970
        let baseline = FrameStamp(
            frameID: "probe-frame-0",
            arbiterInstance: model.arbiterInstance,
            controllerEpoch: 0,
            geometryGeneration: 1,
            capturedAt: now
        )
        _ = model.activate(frame: baseline)
        _ = model.acquireAgentControl(frameID: baseline.frameID, now: now)
        let queuedFrame = FrameStamp(
            frameID: "probe-frame-1",
            arbiterInstance: model.arbiterInstance,
            controllerEpoch: model.controllerEpoch,
            geometryGeneration: model.geometryGeneration,
            capturedAt: now
        )
        _ = model.recordFrame(queuedFrame)
        let seeded = model.enqueue(
            QueuedInputAction(actionID: "probe-action-0", frameID: queuedFrame.frameID, kind: "wait"),
            now: now
        ) == .queued

        let recorder = LockEventRecorder()
        let workspaceCenter = NSWorkspace.shared.notificationCenter
        let workspaceObservers = [
            workspaceCenter.addObserver(forName: NSWorkspace.willSleepNotification, object: nil, queue: .main) { _ in
                recorder.record("willSleep", source: "NSWorkspace")
            },
            workspaceCenter.addObserver(forName: NSWorkspace.didWakeNotification, object: nil, queue: .main) { _ in
                recorder.record("didWake", source: "NSWorkspace")
            },
            workspaceCenter.addObserver(forName: NSWorkspace.screensDidSleepNotification, object: nil, queue: .main) { _ in
                recorder.record("screensDidSleep", source: "NSWorkspace")
            },
            workspaceCenter.addObserver(forName: NSWorkspace.screensDidWakeNotification, object: nil, queue: .main) { _ in
                recorder.record("screensDidWake", source: "NSWorkspace")
            },
            workspaceCenter.addObserver(forName: NSWorkspace.sessionDidResignActiveNotification, object: nil, queue: .main) { _ in
                recorder.record("sessionDidResignActive", source: "NSWorkspace")
            },
            workspaceCenter.addObserver(forName: NSWorkspace.sessionDidBecomeActiveNotification, object: nil, queue: .main) { _ in
                recorder.record("sessionDidBecomeActive", source: "NSWorkspace")
            },
        ]
        let distributedCenter = DistributedNotificationCenter.default()
        let distributedObservers = [
            distributedCenter.addObserver(
                forName: Notification.Name("com.apple.screenIsLocked"),
                object: nil,
                queue: .main
            ) { _ in
                recorder.record("screenIsLocked", source: "DistributedNotificationCenter")
            },
            distributedCenter.addObserver(
                forName: Notification.Name("com.apple.screenIsUnlocked"),
                object: nil,
                queue: .main
            ) { _ in
                recorder.record("screenIsUnlocked", source: "DistributedNotificationCenter")
            },
            distributedCenter.addObserver(
                forName: Notification.Name("com.apple.screensaver.didstart"), object: nil, queue: .main
            ) { _ in
                recorder.record("screenSaverDidStart", source: "DistributedNotificationCenter")
            },
            distributedCenter.addObserver(
                forName: Notification.Name("com.apple.screensaver.didstop"), object: nil, queue: .main
            ) { _ in
                recorder.record("screenSaverDidStop", source: "DistributedNotificationCenter")
            },
        ]

        let initial = snapshot()
        var summaries: [ControllerTransitionSummary] = []
        if let reason = suspensionReason(forSnapshot: initial) {
            summaries.append(ControllerTransitionSummary(transition: model.suspend(reason: reason)))
        }
        let deadline = Date().addingTimeInterval(max(0, duration))
        while Date() < deadline {
            RunLoop.current.run(mode: .default, before: min(deadline, Date().addingTimeInterval(0.1)))
        }
        let final = snapshot()

        for observer in workspaceObservers {
            workspaceCenter.removeObserver(observer)
        }
        for observer in distributedObservers {
            distributedCenter.removeObserver(observer)
        }

        // Apply the state machine for every observed suspension-class transition. Output only.
        for event in recorder.events {
            guard let reason = suspensionReason(for: event.kind) else { continue }
            let transition = model.suspend(reason: reason)
            summaries.append(ControllerTransitionSummary(transition: transition))
            _ = model.noteSystemReturn()
        }
        if model.state == .active, let reason = suspensionReason(forSnapshot: final) {
            summaries.append(ControllerTransitionSummary(transition: model.suspend(reason: reason)))
        }
        let replayAttempt = model.drainForExecution(frameID: queuedFrame.frameID, now: Date().timeIntervalSince1970)
        let replayBlocked = seeded && !summaries.isEmpty && replayAttempt.isEmpty
            && model.pendingActionCount == 0 && model.discardedActionCount > 0

        let observedKinds = recorder.events.map { $0.kind }
        return LockStateSection(
            initial: initial,
            final: final,
            events: recorder.events,
            observation_seconds: max(0, duration),
            os_transitions_observed: observedKinds,
            model_transitions_applied: summaries,
            queue_replay_blocked: replayBlocked,
            evidence_class: observedKinds.isEmpty
                ? "os_observation:none_in_window + state_machine_spike"
                : "os_observation:transitions_in_window + state_machine_spike",
            note: "Observation only: the probe never locks, sleeps or wakes the machine. Any transition in "
                + "os_transitions_observed is OS evidence for this host and window; model_transitions_applied "
                + "and queue_replay_blocked are the state-machine spike and are NOT OS evidence about macOS "
                + "behavior during lock or wake. Initial/final locked, off-console or unknown snapshots also "
                + "suspend the model; queue_replay_blocked requires an admitted model action and actual discard."
        )
    }

    static func snapshot() -> LockStateSnapshot {
        guard let dictionary = CGSessionCopyCurrentDictionary() as? [String: Any] else {
            return LockStateSnapshot(
                on_console: nil,
                screen_locked: nil,
                session_dictionary_keys: [],
                note: "CGSessionCopyCurrentDictionary unavailable (no GUI session for this process)"
            )
        }
        let interestingKeys = dictionary.keys
            .filter { $0.contains("Lock") || $0.contains("Console") || $0.contains("Login") }
            .sorted()
        return LockStateSnapshot(
            on_console: dictionary["kCGSSessionOnConsoleKey"] as? Bool,
            screen_locked: dictionary["CGSSessionScreenIsLocked"] as? Bool,
            session_dictionary_keys: interestingKeys,
            note: "values come from the current GUI session dictionary; nil means the key was absent"
        )
    }

    private static func suspensionReason(forSnapshot snapshot: LockStateSnapshot) -> SuspensionReason? {
        if snapshot.screen_locked == true { return .screenLock }
        if snapshot.on_console == false { return .fastUserSwitch }
        if snapshot.on_console != true || snapshot.screen_locked == nil { return .unknown }
        return nil
    }

    private static func suspensionReason(for eventKind: String) -> SuspensionReason? {
        switch eventKind {
        case "screenIsLocked":
            return .screenLock
        case "screenSaverDidStart":
            return .screenSaver
        case "screensDidSleep", "willSleep":
            return .sleep
        case "sessionDidResignActive":
            return .fastUserSwitch
        default:
            return nil
        }
    }
}
