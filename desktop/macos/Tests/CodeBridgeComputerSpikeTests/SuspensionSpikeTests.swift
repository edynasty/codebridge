import XCTest

@testable import CodeBridgeComputerSpike

final class SuspensionModelTests: XCTestCase {
    private let arbiter = "arb-1"

    private func makeModel() -> ComputerSuspensionModel {
        ComputerSuspensionModel(arbiterInstance: arbiter, maxFrameAge: 30)
    }

    private func frame(
        id: String,
        epoch: Int,
        geometry: Int,
        capturedAt: TimeInterval,
        arbiter: String = "arb-1"
    ) -> FrameStamp {
        FrameStamp(
            frameID: id,
            arbiterInstance: arbiter,
            controllerEpoch: epoch,
            geometryGeneration: geometry,
            capturedAt: capturedAt
        )
    }

    func testQueuedInputIsDiscardedOnSuspensionAndCannotReplay() {
        let model = makeModel()
        let t0: TimeInterval = 1_000
        XCTAssertEqual(model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0)), .valid)
        XCTAssertEqual(model.acquireAgentControl(frameID: "f1", now: t0), .granted(epoch: 1))

        // Acquisition is a controller change, so the agent must observe again before acting.
        let planned = frame(id: "f2", epoch: model.controllerEpoch, geometry: model.geometryGeneration, capturedAt: t0)
        XCTAssertEqual(model.recordFrame(planned), .valid)
        XCTAssertEqual(
            model.enqueue(QueuedInputAction(actionID: "a1", frameID: "f2", kind: "click"), now: t0),
            .queued
        )
        XCTAssertEqual(model.pendingActionCount, 1)

        let transition = model.suspend(reason: .screenLock)
        XCTAssertEqual(transition.controller, .none)
        XCTAssertEqual(transition.previousController, .agent)
        XCTAssertEqual(transition.discardedActions, 1)
        XCTAssertGreaterThan(transition.controllerEpoch, 1)
        XCTAssertEqual(model.pendingActionCount, 0)

        // After the system returns: still suspended, nothing drains, the pre-lock frame is gone.
        XCTAssertEqual(model.noteSystemReturn(), .stillSuspendedAwaitingFreshObservation)
        XCTAssertTrue(model.drainForExecution(frameID: "f2", now: t0 + 60).isEmpty)
        XCTAssertNotEqual(model.validate(frameID: "f2", now: t0 + 60), .valid)
        XCTAssertEqual(model.controller, .none)
        XCTAssertEqual(model.discardedActionCount, 1)
    }

    func testResumeRequiresAFreshObservationFrame() {
        let model = makeModel()
        let t0: TimeInterval = 2_000
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        _ = model.acquireAgentControl(frameID: "f1", now: t0)
        _ = model.suspend(reason: .sleep)

        // Reusing the pre-suspension frame is rejected: the epoch and geometry moved.
        let stale = frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0)
        guard case .rejected = model.resumeAfterFreshObservation(frame: stale, humanActor: false, now: t0 + 5) else {
            return XCTFail("a pre-suspension frame must not resume the session")
        }

        // A fresh observation taken after the return succeeds for a non-human-hold suspension.
        let fresh = frame(
            id: "f2",
            epoch: model.controllerEpoch,
            geometry: model.geometryGeneration,
            capturedAt: t0 + 10
        )
        let outcome = model.resumeAfterFreshObservation(frame: fresh, humanActor: false, now: t0 + 10)
        guard case .granted = outcome else {
            return XCTFail("a fresh frame must resume the session, got \(outcome)")
        }
        XCTAssertEqual(model.state, .active)
        XCTAssertEqual(model.controller, .none)
        XCTAssertEqual(model.acquireAgentControl(frameID: "f2", now: t0 + 10), .granted(epoch: model.controllerEpoch))
    }

    func testGeometryChangeInvalidatesFramesPlannedAgainstTheOldGeometry() {
        let model = makeModel()
        let t0: TimeInterval = 3_000
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        _ = model.acquireAgentControl(frameID: "f1", now: t0)
        _ = model.suspend(reason: .displayChange)
        XCTAssertEqual(model.geometryGeneration, 2)

        // A frame stamped with the old geometry generation is rejected even though the epoch is current.
        let oldGeometry = frame(
            id: "f2",
            epoch: model.controllerEpoch,
            geometry: 1,
            capturedAt: t0 + 1
        )
        XCTAssertEqual(model.recordFrame(oldGeometry), .geometryChanged)
    }

    func testControllerEpochChangeInvalidatesFrames() {
        let model = makeModel()
        let t0: TimeInterval = 4_000
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        _ = model.acquireAgentControl(frameID: "f1", now: t0)
        let epochBefore = model.controllerEpoch
        _ = model.suspend(reason: .ipcLoss)
        XCTAssertGreaterThan(model.controllerEpoch, epochBefore)

        let oldEpoch = frame(id: "f2", epoch: epochBefore, geometry: model.geometryGeneration, capturedAt: t0 + 1)
        XCTAssertEqual(model.recordFrame(oldEpoch), .staleController)
    }

    func testHumanHoldRequiresAHumanActorToResume() {
        let model = makeModel()
        let t0: TimeInterval = 5_000
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        _ = model.acquireAgentControl(frameID: "f1", now: t0)
        let transition = model.suspend(reason: .emergencyStop)
        XCTAssertTrue(transition.requiresHumanResume)
        XCTAssertEqual(model.noteSystemReturn(), .requiresHumanResume)

        let fresh = frame(
            id: "f2",
            epoch: model.controllerEpoch,
            geometry: model.geometryGeneration,
            capturedAt: t0 + 5
        )
        guard case .rejected(let reason) = model.resumeAfterFreshObservation(frame: fresh, humanActor: false, now: t0 + 5) else {
            return XCTFail("the model path must not leave human hold")
        }
        XCTAssertEqual(reason, "human_hold")

        guard case .granted = model.resumeAfterFreshObservation(frame: fresh, humanActor: true, now: t0 + 5) else {
            return XCTFail("a human actor must be able to resume")
        }
        XCTAssertFalse(model.humanHold)
    }

    func testStaleFrameIsRejectedAfterTheAgeBound() {
        let model = makeModel()
        let t0: TimeInterval = 6_000
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        _ = model.acquireAgentControl(frameID: "f1", now: t0)
        let planned = frame(id: "f2", epoch: model.controllerEpoch, geometry: model.geometryGeneration, capturedAt: t0)
        XCTAssertEqual(model.recordFrame(planned), .valid)
        let validation = model.validate(frameID: "f2", now: t0 + 31)
        guard case .expired(let age) = validation else {
            return XCTFail("expected the frame to expire, got \(validation)")
        }
        XCTAssertEqual(age, 31)
        XCTAssertEqual(validation.rejectionReason, "stale_frame")
        XCTAssertTrue(model.drainForExecution(frameID: "f2", now: t0 + 31).isEmpty)
    }

    func testAcquisitionBumpsTheEpochSoTheAcquisitionFrameCannotBeReused() {
        let model = makeModel()
        let t0: TimeInterval = 6_500
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        XCTAssertEqual(model.acquireAgentControl(frameID: "f1", now: t0), .granted(epoch: 1))
        XCTAssertEqual(
            model.enqueue(QueuedInputAction(actionID: "a1", frameID: "f1", kind: "click"), now: t0),
            .rejectedFrameInvalid(.staleController)
        )
    }

    func testEnqueueIsRefusedWithoutAgentControl() {
        let model = makeModel()
        let t0: TimeInterval = 7_000
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        XCTAssertEqual(
            model.enqueue(QueuedInputAction(actionID: "a1", frameID: "f1", kind: "click"), now: t0),
            .rejectedNoAgentController
        )
        _ = model.suspend(reason: .screenLock)
        XCTAssertEqual(
            model.enqueue(QueuedInputAction(actionID: "a2", frameID: "f1", kind: "click"), now: t0),
            .rejectedNotActive
        )
    }

    func testArbiterRestartInvalidatesForeignFrames() {
        let model = makeModel()
        let t0: TimeInterval = 8_000
        _ = model.activate(frame: frame(id: "f1", epoch: 0, geometry: 1, capturedAt: t0))
        let foreign = frame(id: "f2", epoch: 0, geometry: 1, capturedAt: t0, arbiter: "other-arbiter")
        XCTAssertEqual(model.recordFrame(foreign), .arbiterRestarted)
    }
}

final class InputClassifierTests: XCTestCase {
    func testOnlyEngineOriginatedEventsAreIgnored() {
        let engine: Int32 = 4_242
        let engineSample = InputEventSample(sourceProcessID: engine, eventTypeRawValue: 1, secureInputEnabled: false)
        XCTAssertEqual(PhysicalInputClassifier.origin(sample: engineSample, engineProcessID: engine), .engineInjected)
        XCTAssertEqual(
            PhysicalInputClassifier.decision(sample: engineSample, engineProcessID: engine),
            .ignoreEngineEvent
        )

        let hardware = InputEventSample(sourceProcessID: 0, eventTypeRawValue: 1, secureInputEnabled: false)
        XCTAssertEqual(PhysicalInputClassifier.origin(sample: hardware, engineProcessID: engine), .physicalHardware)
        XCTAssertEqual(
            PhysicalInputClassifier.decision(sample: hardware, engineProcessID: engine),
            .preempt(reason: "physical_input")
        )

        let otherProcess = InputEventSample(sourceProcessID: 999, eventTypeRawValue: 1, secureInputEnabled: false)
        XCTAssertEqual(
            PhysicalInputClassifier.origin(sample: otherProcess, engineProcessID: engine),
            .syntheticOtherProcess
        )
        XCTAssertEqual(
            PhysicalInputClassifier.decision(sample: otherProcess, engineProcessID: engine),
            .preempt(reason: "unknown_synthetic_input")
        )
    }

    func testUnknownOriginNeverResolvesToSafe() {
        let unknown = InputEventSample(sourceProcessID: -1, eventTypeRawValue: 1, secureInputEnabled: true)
        XCTAssertEqual(PhysicalInputClassifier.origin(sample: unknown, engineProcessID: 1), .unknown)
        XCTAssertEqual(
            PhysicalInputClassifier.decision(sample: unknown, engineProcessID: 1),
            .preempt(reason: "unknown_input_origin")
        )
    }

    func testInputAvailabilityFollowsTheMonitorState() {
        XCTAssertTrue(InputAvailabilityPolicy.inputIsAvailable(monitor: .verifiedRunning, secureInputEnabled: false))
        XCTAssertFalse(InputAvailabilityPolicy.inputIsAvailable(monitor: .unavailable, secureInputEnabled: false))
        XCTAssertFalse(InputAvailabilityPolicy.inputIsAvailable(monitor: .installedUnverified, secureInputEnabled: false))
        XCTAssertFalse(InputAvailabilityPolicy.inputIsAvailable(monitor: .verifiedRunningButDisabled, secureInputEnabled: false))
    }

    func testSecureInputBlocksAnOtherwiseVerifiedMonitor() {
        XCTAssertFalse(InputAvailabilityPolicy.inputIsAvailable(monitor: .verifiedRunning, secureInputEnabled: true))
    }
}
