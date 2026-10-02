import Foundation

/// Host IPC protocol version.
///
/// Contract: `docs/v2/provider-contracts.md` §7 (length-prefixed JSON-RPC 2.0 over a Unix-domain
/// socket in a 0700 per-user directory, `major.minor` handshake, N and N-1 minor interop) and
/// `docs/v2/architecture.md` §5.6 (version skew: a major mismatch disables the app's services).
public struct ProtocolVersion: Hashable, Codable, Sendable, Comparable, CustomStringConvertible {
    /// Protocol version this build speaks.
    public static let current = ProtocolVersion(major: 1, minor: 0)

    public var major: Int
    public var minor: Int

    public init(major: Int, minor: Int) {
        self.major = major
        self.minor = minor
    }

    public var description: String { "\(major).\(minor)" }

    public static func < (lhs: ProtocolVersion, rhs: ProtocolVersion) -> Bool {
        lhs.major == rhs.major ? lhs.minor < rhs.minor : lhs.major < rhs.major
    }
}

/// Outcome of the `host.hello` version negotiation (daemon-owned rule, see `HostIPCErrorCode`).
public enum ProtocolNegotiation: Equatable, Sendable {
    case accepted(negotiated: ProtocolVersion)
    case rejectedMajorMismatch(client: ProtocolVersion, daemon: ProtocolVersion)
    case rejectedMinorUnsupported(client: ProtocolVersion, daemon: ProtocolVersion)

    public var isAccepted: Bool {
        if case .accepted = self { return true }
        return false
    }

    public var negotiated: ProtocolVersion? {
        if case .accepted(let version) = self { return version }
        return nil
    }

    public var failureDescription: String? {
        switch self {
        case .accepted:
            return nil
        case .rejectedMajorMismatch(let client, let daemon):
            return "protocol major mismatch (client \(client), daemon \(daemon))"
        case .rejectedMinorUnsupported(let client, let daemon):
            return "protocol minor unsupported (client \(client), daemon \(daemon))"
        }
    }
}

/// Version rules shared with `codebridged` (frozen 2026-10-02 with the daemon side).
public enum ProtocolRules {
    /// The daemon accepts a client whose minor is `daemon.minor` or `daemon.minor - 1`.
    /// Negotiated minor is the lower of the two.
    public static func negotiate(client: ProtocolVersion, daemon: ProtocolVersion) -> ProtocolNegotiation {
        guard client.major == daemon.major, daemon.major == ProtocolVersion.current.major else {
            return .rejectedMajorMismatch(client: client, daemon: daemon)
        }
        let supportedMinors = [daemon.minor, daemon.minor - 1]
        guard supportedMinors.contains(client.minor) else {
            return .rejectedMinorUnsupported(client: client, daemon: daemon)
        }
        return .accepted(negotiated: ProtocolVersion(major: client.major, minor: min(client.minor, daemon.minor)))
    }
}
