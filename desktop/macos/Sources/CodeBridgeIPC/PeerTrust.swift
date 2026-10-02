import Foundation
import Security

/// Apple-anchored code requirement used for **authorization** (not just introspection).
///
/// `anchor apple generic` means the peer must carry a genuine Apple-issued certificate chain
/// (Developer ID / Apple Development), so an ad-hoc or unsigned peer can never satisfy it.
public enum PeerRequirement {
    public static func make(identifier: String, teamIdentifier: String?) -> SecRequirement? {
        let safeIdentifier = sanitize(identifier)
        guard !safeIdentifier.isEmpty else { return nil }
        var clause = "anchor apple generic and identifier \"\(safeIdentifier)\""
        if let team = teamIdentifier.map(sanitize), !team.isEmpty {
            clause += " and certificate leaf[subject.OU] = \"\(team)\""
        }
        var requirement: SecRequirement?
        let status = SecRequirementCreateWithString(clause as CFString, [], &requirement)
        guard status == errSecSuccess, let requirement = requirement else { return nil }
        return requirement
    }

    public static func describe(identifier: String, teamIdentifier: String?) -> String {
        var clause = "anchor apple generic and identifier \"\(sanitize(identifier))\""
        if let team = teamIdentifier.map(sanitize), !team.isEmpty {
            clause += " and certificate leaf[subject.OU] = \"\(team)\""
        }
        return clause
    }

    private static func sanitize(_ value: String) -> String {
        value
            .replacingOccurrences(of: "\"", with: "")
            .replacingOccurrences(of: "\n", with: "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

/// Whether this client may proceed with a peer, and how strictly the peer must be verified.
public enum PeerTrustDecision: Equatable, Sendable {
    /// The peer signature must be verified with an Apple-anchored requirement before any call.
    case requireVerifiedPeer(ExpectedPeerSignature)
    /// Phase 0 diagnostics only: the local pre-check may be skipped (peer UID is still enforced).
    /// Harness peers are only acceptable for services that carry no OS authority.
    case allowUnverifiedDiagnosticsPeer(reason: String)
    /// Refuse to continue.
    case refuse(reason: String)

    public var isRefusal: Bool {
        if case .refuse = self { return true }
        return false
    }

    public var reason: String {
        switch self {
        case .requireVerifiedPeer(let expected):
            return "verified peer required: \(PeerRequirement.describe(identifier: expected.signingIdentifier, teamIdentifier: expected.teamIdentifier))"
        case .allowUnverifiedDiagnosticsPeer(let reason):
            return reason
        case .refuse(let reason):
            return reason
        }
    }
}

/// Client-side peer trust policy.
///
/// Frozen rules (parent integration security finding, 2026-10-02):
/// - `role = app` **never** accepts an unverified or ad-hoc peer, with or without the environment
///   opt-in; it additionally requires a configured daemon signing identifier **and** a non-empty
///   Team ID, so an unsigned same-user process cannot impersonate the daemon to a signed app.
/// - the unverified exception exists only for Phase 0 `diagnostics` (and `harness`, which receives
///   no OS services); everything else fails closed.
public enum PeerTrustPolicy {
    public static func decide(
        role: String,
        expectedDaemon: ExpectedPeerSignature?,
        allowUnverifiedPeer: Bool
    ) -> PeerTrustDecision {
        let identifier = expectedDaemon?.signingIdentifier ?? ""
        let team = (expectedDaemon?.teamIdentifier ?? "").trimmingCharacters(in: .whitespacesAndNewlines)

        if role == HostIPCRole.app {
            guard !identifier.isEmpty else {
                return .refuse(
                    reason: "role=app requires CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID; refusing to connect to an "
                        + "unidentified daemon (the environment opt-in does not apply to the app role)"
                )
            }
            guard !team.isEmpty else {
                return .refuse(
                    reason: "role=app requires a non-empty Team ID (CODEBRIDGE_EXPECT_TEAM_ID); ad-hoc and "
                        + "teamless peers are not trusted"
                )
            }
            return .requireVerifiedPeer(ExpectedPeerSignature(signingIdentifier: identifier, teamIdentifier: team))
        }

        if !identifier.isEmpty {
            // A configured expectation always applies, whatever the role.
            if team.isEmpty && (role == HostIPCRole.harness || role == HostIPCRole.diagnostics) {
                return .requireVerifiedPeer(ExpectedPeerSignature(signingIdentifier: identifier, teamIdentifier: nil))
            }
            return .requireVerifiedPeer(ExpectedPeerSignature(signingIdentifier: identifier, teamIdentifier: team.isEmpty ? nil : team))
        }

        switch role {
        case HostIPCRole.diagnostics:
            guard allowUnverifiedPeer else {
                return .refuse(
                    reason: "no expected daemon signing identity configured; set "
                        + "CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID (+ CODEBRIDGE_EXPECT_TEAM_ID) or opt in with "
                        + "\(HostIPCPaths.allowUnverifiedDaemonPeerEnvironmentKey)=1 for Phase 0 diagnostics"
                )
            }
            return .allowUnverifiedDiagnosticsPeer(
                reason: "Phase 0 diagnostics: peer UID is enforced; the daemon still enforces its own rule and grants "
                    + "this caller class no OS-capable services"
            )
        case HostIPCRole.harness:
            guard allowUnverifiedPeer else {
                return .refuse(
                    reason: "harness peers without a configured signing identity require "
                        + "\(HostIPCPaths.allowUnverifiedDaemonPeerEnvironmentKey)=1"
                )
            }
            return .allowUnverifiedDiagnosticsPeer(
                reason: "Phase 0 harness: peer UID is enforced; harness callers receive no OS-capable services"
            )
        default:
            return .refuse(reason: "role=\(role) requires a configured, verified daemon signing identity")
        }
    }
}
