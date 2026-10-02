import XCTest

@testable import CodeBridgeIPC

final class FramingTests: XCTestCase {
    func testJSONFrameRoundTrip() throws {
        let payload = Data(#"{"jsonrpc":"2.0","id":1,"method":"host.hello"}"#.utf8)
        let encoded = try FrameCodec.encode(payload, kind: .json)
        XCTAssertEqual(encoded.count, payload.count + FrameCodec.headerLength)
        XCTAssertEqual(encoded[encoded.startIndex] & 0x80, 0x00)

        var decoder = FrameDecoder()
        decoder.append(encoded)
        let frame = try XCTUnwrap(try decoder.nextFrame())
        XCTAssertEqual(frame.kind, .json)
        XCTAssertEqual(frame.payload, payload)
        XCTAssertNil(try decoder.nextFrame())
    }

    func testBinaryAttachmentFrameRoundTrip() throws {
        let meta = AttachmentMeta(
            attachmentID: "att_1",
            mime: "image/png",
            bytes: 5,
            sha256: "aa",
            role: "observation"
        )
        let body = Data([1, 2, 3, 4, 5])
        let encoded = try FrameCodec.encodeAttachment(meta: meta, payload: body)
        XCTAssertEqual(encoded[encoded.startIndex] & 0x80, 0x80)

        var decoder = FrameDecoder()
        decoder.append(encoded)
        let frame = try XCTUnwrap(try decoder.nextFrame())
        XCTAssertEqual(frame.kind, .binaryAttachment)
        let decoded = try FrameCodec.decodeAttachment(frame.payload)
        XCTAssertEqual(decoded.meta.attachmentID, "att_1")
        XCTAssertEqual(decoded.meta.mime, "image/png")
        XCTAssertEqual(decoded.meta.bytes, 5)
        XCTAssertEqual(decoded.body, body)
    }

    func testControlFramesOverOneMiBillionAreRejected() {
        let oversized = Data(repeating: 0x20, count: FrameCodec.maxJSONFrameBytes + 1)
        XCTAssertThrowsError(try FrameCodec.encode(oversized, kind: .json)) { error in
            XCTAssertEqual(error as? FramingError, .frameTooLarge(kind: .json, bytes: oversized.count, limit: FrameCodec.maxJSONFrameBytes))
        }
    }

    func testAttachmentFramesAreBoundedAtSixteenMiB() {
        // header 0xFFFFFFFF: attachment flag plus a 0x7FFFFFFF byte length, far beyond the limit
        let header = Data([0xFF, 0xFF, 0xFF, 0xFF])
        XCTAssertThrowsError(try FrameCodec.decodeHeader(header)) { error in
            XCTAssertEqual(
                error as? FramingError,
                .frameTooLarge(kind: .binaryAttachment, bytes: 0x7FFF_FFFF, limit: FrameCodec.maxAttachmentFrameBytes)
            )
        }
    }

    func testDecoderWaitsForCompleteFrames() throws {
        let payload = Data("hello".utf8)
        let encoded = try FrameCodec.encode(payload, kind: .json)
        var decoder = FrameDecoder()
        decoder.append(encoded.prefix(FrameCodec.headerLength + 2))
        XCTAssertNil(try decoder.nextFrame())
        decoder.append(encoded.suffix(from: encoded.index(encoded.startIndex, offsetBy: FrameCodec.headerLength + 2)))
        let frame = try XCTUnwrap(try decoder.nextFrame())
        XCTAssertEqual(frame.payload, payload)
    }

    func testDecoderHandlesTwoFramesInOneBuffer() throws {
        let first = try FrameCodec.encode(Data("one".utf8))
        let second = try FrameCodec.encode(Data("two".utf8))
        var decoder = FrameDecoder()
        decoder.append(first)
        decoder.append(second)
        XCTAssertEqual(try decoder.nextFrame()?.payload, Data("one".utf8))
        XCTAssertEqual(try decoder.nextFrame()?.payload, Data("two".utf8))
        XCTAssertNil(try decoder.nextFrame())
    }
}

final class PeerTrustPolicyTests: XCTestCase {
    func testRoleAppCannotBypassVerificationEvenWithTheOptIn() {
        let decision = PeerTrustPolicy.decide(
            role: HostIPCRole.app,
            expectedDaemon: nil,
            allowUnverifiedPeer: true
        )
        XCTAssertTrue(decision.isRefusal)
    }

    func testRoleAppRequiresANonEmptyTeamIdentifier() {
        let decision = PeerTrustPolicy.decide(
            role: HostIPCRole.app,
            expectedDaemon: ExpectedPeerSignature(signingIdentifier: "com.codebridge.daemon", teamIdentifier: ""),
            allowUnverifiedPeer: true
        )
        XCTAssertTrue(decision.isRefusal)
    }

    func testRoleAppWithAFullExpectationRequiresALiveVerifiedPeer() {
        let expected = ExpectedPeerSignature(signingIdentifier: "com.codebridge.daemon", teamIdentifier: "TEAMID123")
        let decision = PeerTrustPolicy.decide(
            role: HostIPCRole.app,
            expectedDaemon: expected,
            allowUnverifiedPeer: false
        )
        XCTAssertEqual(decision, .requireVerifiedPeer(expected))
    }

    func testDiagnosticsMaySkipItsPreCheckOnlyWithExplicitOptIn() {
        let withOptIn = PeerTrustPolicy.decide(
            role: HostIPCRole.diagnostics,
            expectedDaemon: nil,
            allowUnverifiedPeer: true
        )
        if case .allowUnverifiedDiagnosticsPeer = withOptIn {} else {
            XCTFail("diagnostics with the explicit opt-in must be allowed, got \(withOptIn)")
        }

        let withoutOptIn = PeerTrustPolicy.decide(
            role: HostIPCRole.diagnostics,
            expectedDaemon: nil,
            allowUnverifiedPeer: false
        )
        XCTAssertTrue(withoutOptIn.isRefusal)
    }

    func testHarnessMaySkipPeerCheckOnlyWithExplicitOptIn() {
        let decision = PeerTrustPolicy.decide(
            role: HostIPCRole.harness,
            expectedDaemon: nil,
            allowUnverifiedPeer: true
        )
        if case .allowUnverifiedDiagnosticsPeer = decision {} else {
            XCTFail("harness opt-in was rejected: \(decision)")
        }
        let withoutOptIn = PeerTrustPolicy.decide(
            role: HostIPCRole.harness, expectedDaemon: nil, allowUnverifiedPeer: false
        )
        XCTAssertTrue(withoutOptIn.isRefusal)
    }

    func testAConfiguredExpectationAlwaysApplies() {
        let expected = ExpectedPeerSignature(signingIdentifier: "com.codebridge.daemon", teamIdentifier: nil)
        let decision = PeerTrustPolicy.decide(
            role: HostIPCRole.diagnostics,
            expectedDaemon: expected,
            allowUnverifiedPeer: true
        )
        XCTAssertEqual(decision, .requireVerifiedPeer(expected))
    }

    func testUnknownRoleRequiresAConfiguredIdentity() {
        let decision = PeerTrustPolicy.decide(role: "local_ui", expectedDaemon: nil, allowUnverifiedPeer: true)
        XCTAssertTrue(decision.isRefusal)
    }

    func testEmptyIdentifierCannotProduceARequirement() {
        XCTAssertNil(PeerRequirement.make(identifier: "", teamIdentifier: "TEAMID123"))
        XCTAssertNil(PeerRequirement.make(identifier: "   ", teamIdentifier: nil))
    }

}

final class ProtocolNegotiationTests: XCTestCase {
    func testIdenticalVersionsAreAccepted() {
        let client = ProtocolVersion(major: 1, minor: 0)
        let daemon = ProtocolVersion(major: 1, minor: 0)
        XCTAssertEqual(
            ProtocolRules.negotiate(client: client, daemon: daemon),
            .accepted(negotiated: ProtocolVersion(major: 1, minor: 0))
        )
    }

    func testOlderClientAgainstNewerDaemonIsAcceptedWithLowerMinor() {
        let client = ProtocolVersion(major: 1, minor: 0)
        let daemon = ProtocolVersion(major: 1, minor: 1)
        XCTAssertEqual(
            ProtocolRules.negotiate(client: client, daemon: daemon),
            .accepted(negotiated: ProtocolVersion(major: 1, minor: 0))
        )
    }

    func testNewerClientAgainstOlderDaemonIsRejected() {
        let client = ProtocolVersion(major: 1, minor: 1)
        let daemon = ProtocolVersion(major: 1, minor: 0)
        XCTAssertEqual(
            ProtocolRules.negotiate(client: client, daemon: daemon),
            .rejectedMinorUnsupported(client: client, daemon: daemon)
        )
    }

    func testMinorTwoAheadIsRejected() {
        let client = ProtocolVersion(major: 1, minor: 2)
        let daemon = ProtocolVersion(major: 1, minor: 0)
        XCTAssertEqual(
            ProtocolRules.negotiate(client: client, daemon: daemon),
            .rejectedMinorUnsupported(client: client, daemon: daemon)
        )
    }

    func testMajorMismatchIsRejected() {
        let client = ProtocolVersion(major: 2, minor: 0)
        let daemon = ProtocolVersion(major: 1, minor: 0)
        XCTAssertEqual(
            ProtocolRules.negotiate(client: client, daemon: daemon),
            .rejectedMajorMismatch(client: client, daemon: daemon)
        )
        XCTAssertFalse(ProtocolRules.negotiate(client: client, daemon: daemon).isAccepted)
    }


    func testJSONValueRoundTripsThroughJSON() throws {
        let value = JSONValue.object([
            "protocol": .object(["major": .int(1), "minor": .int(0)]),
            "accepted": .bool(true),
            "capabilities": .array([.string("computer"), .string("approval")]),
            "reason": .null,
        ])
        let encoded = try value.encoded()
        let decoded = try JSONValue.decode(from: encoded)
        XCTAssertEqual(decoded, value)
        XCTAssertEqual(decoded["protocol"]?["major"]?.intValue, 1)
        XCTAssertEqual(decoded["capabilities"]?.arrayValue?.count, 2)
    }

}
