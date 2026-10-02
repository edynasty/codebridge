import Darwin
import Foundation
import Security

/// Peer credentials of a Unix-domain socket peer.
public struct PeerCredentials: Equatable, Sendable, CustomStringConvertible {
    public var pid: pid_t
    public var uid: uid_t
    public var gid: gid_t
    public var auditToken: Data?

    public init(pid: pid_t, uid: uid_t, gid: gid_t, auditToken: Data?) {
        self.pid = pid
        self.uid = uid
        self.gid = gid
        self.auditToken = auditToken
    }

    public var description: String {
        "pid=\(pid) uid=\(uid) gid=\(gid) auditToken=\(auditToken == nil ? "unavailable" : "present")"
    }
}

/// Live code-signing identity of a process, read through the Security framework.
public struct SigningIdentity: Equatable, Sendable, CustomStringConvertible {
    public var pid: pid_t?
    public var signingIdentifier: String?
    public var teamIdentifier: String?
    public var cdhashHex: String?
    public var isAdHoc: Bool
    public var isValid: Bool
    public var executablePath: String?
    public var entitlements: [String: JSONValue]?

    public init(
        pid: pid_t? = nil,
        signingIdentifier: String? = nil,
        teamIdentifier: String? = nil,
        cdhashHex: String? = nil,
        isAdHoc: Bool = false,
        isValid: Bool = false,
        executablePath: String? = nil,
        entitlements: [String: JSONValue]? = nil
    ) {
        self.pid = pid
        self.signingIdentifier = signingIdentifier
        self.teamIdentifier = teamIdentifier
        self.cdhashHex = cdhashHex
        self.isAdHoc = isAdHoc
        self.isValid = isValid
        self.executablePath = executablePath
        self.entitlements = entitlements
    }

    public var description: String {
        "identifier=\(signingIdentifier ?? "none") team=\(teamIdentifier ?? "none") "
            + "adhoc=\(isAdHoc) valid=\(isValid) cdhash=\(cdhashHex ?? "none")"
    }
}

public struct ExpectedPeerSignature: Equatable, Sendable {
    public var signingIdentifier: String
    public var teamIdentifier: String?

    public init(signingIdentifier: String, teamIdentifier: String? = nil) {
        self.signingIdentifier = signingIdentifier
        self.teamIdentifier = teamIdentifier
    }
}

public enum SignatureVerification: Equatable, Sendable {
    case verified(SigningIdentity)
    case identityMismatch(expected: ExpectedPeerSignature, actual: SigningIdentity, osStatus: Int32)
    case unverifiable(reason: String, actual: SigningIdentity?)

    public var isVerified: Bool {
        if case .verified = self { return true }
        return false
    }

    public var summary: String {
        switch self {
        case .verified(let identity):
            return "verified: \(identity)"
        case .identityMismatch(let expected, let actual, let status):
            return "identity mismatch (OSStatus \(status)): expected "
                + "\(PeerRequirement.describe(identifier: expected.signingIdentifier, teamIdentifier: expected.teamIdentifier)) "
                + "got \(actual)"
        case .unverifiable(let reason, let actual):
            return "unverifiable: \(reason) (actual: \(actual.map { "\($0)" } ?? "none"))"
        }
    }
}

/// Peer identity helpers shared by the app and the probe.
///
/// The daemon verifies the app's live signature on its side; this side verifies the daemon's live
/// signature for the same reason (`docs/v2/architecture.md` §13.5).
public enum PeerIdentity {
    public static func credentials(fileDescriptor: Int32) -> PeerCredentials? {
        var pid: pid_t = 0
        var pidLength = socklen_t(MemoryLayout<pid_t>.size)
        guard getsockopt(fileDescriptor, SOL_LOCAL, LOCAL_PEERPID, &pid, &pidLength) == 0 else {
            return nil
        }
        var uid: uid_t = 0
        var gid: gid_t = 0
        guard getpeereid(fileDescriptor, &uid, &gid) == 0 else {
            return nil
        }
        var token = audit_token_t()
        var tokenLength = socklen_t(MemoryLayout<audit_token_t>.size)
        let auditToken: Data? = withUnsafeMutablePointer(to: &token) { pointer -> Data? in
            let status = getsockopt(fileDescriptor, SOL_LOCAL, LOCAL_PEERTOKEN, pointer, &tokenLength)
            guard status == 0 else { return nil }
            return pointer.withMemoryRebound(to: UInt8.self, capacity: MemoryLayout<audit_token_t>.size) { bytes in
                Data(bytes: bytes, count: MemoryLayout<audit_token_t>.size)
            }
        }
        return PeerCredentials(pid: pid, uid: uid, gid: gid, auditToken: auditToken)
    }

    public static func signingIdentity(pid: pid_t) -> SigningIdentity? {
        guard let code = copyGuestCode(attributes: [kSecGuestAttributePid as String: pid]) else { return nil }
        return describe(code: code, pid: pid)
    }

    public static func signingIdentity(auditToken: Data) -> SigningIdentity? {
        guard let code = copyGuestCode(attributes: [kSecGuestAttributeAudit as String: auditToken]) else { return nil }
        return describe(code: code, pid: nil)
    }

    /// Best-effort live identity of a socket peer: audit token when available, PID otherwise.
    public static func inspect(fileDescriptor: Int32) -> SigningIdentity? {
        guard let credentials = credentials(fileDescriptor: fileDescriptor) else { return nil }
        if let token = credentials.auditToken, let identity = signingIdentity(auditToken: token) {
            return identity
        }
        return signingIdentity(pid: credentials.pid)
    }

    /// Authorizes a peer: the **live** `SecCode` obtained from the peer's audit token (preferred) or
    /// PID is checked with `SecCodeCheckValidity` against an Apple-anchored requirement
    /// (`anchor apple generic and identifier … and certificate leaf[subject.OU] = <team>`).
    ///
    /// Static metadata is only used for reporting: an ad-hoc or unsigned peer cannot satisfy an
    /// Apple-anchored requirement, so it can never be authorized.
    public static func verify(fileDescriptor: Int32, expected: ExpectedPeerSignature) -> SignatureVerification {
        guard let credentials = credentials(fileDescriptor: fileDescriptor) else {
            return .unverifiable(reason: "peer credentials unavailable", actual: nil)
        }
        let liveCode: SecCode?
        if let token = credentials.auditToken {
            liveCode = copyGuestCode(attributes: [kSecGuestAttributeAudit as String: token])
        } else {
            liveCode = nil
        }
        let code = liveCode ?? copyGuestCode(attributes: [kSecGuestAttributePid as String: credentials.pid])
        guard let live = code else {
            return .unverifiable(
                reason: "no live code object for peer pid \(credentials.pid)",
                actual: nil
            )
        }
        let identity = describe(code: live, pid: credentials.pid) ?? SigningIdentity(pid: credentials.pid, isValid: false)
        guard let requirement = PeerRequirement.make(
            identifier: expected.signingIdentifier,
            teamIdentifier: expected.teamIdentifier
        ) else {
            return .unverifiable(reason: "could not build a code requirement for the expected identity", actual: identity)
        }
        let status = SecCodeCheckValidity(live, [], requirement)
        if status == errSecSuccess {
            return .verified(identity)
        }
        return .identityMismatch(expected: expected, actual: identity, osStatus: status)
    }

    // MARK: - Private

    private static func copyGuestCode(attributes: [String: Any]) -> SecCode? {
        var code: SecCode?
        let status = SecCodeCopyGuestWithAttributes(nil, attributes as CFDictionary, [], &code)
        guard status == errSecSuccess, let code = code else { return nil }
        return code
    }

    private static func describe(code: SecCode, pid: pid_t?) -> SigningIdentity? {
        var staticCode: SecStaticCode?
        guard SecCodeCopyStaticCode(code, [], &staticCode) == errSecSuccess, let staticCode = staticCode else {
            // A live process without an on-disk signature (for example an unsigned development build).
            return SigningIdentity(pid: pid, isValid: false)
        }
        let valid = SecStaticCodeCheckValidity(staticCode, [], nil) == errSecSuccess
        var information: CFDictionary?
        // kSecCSSigningInformation == 1 << 1 (SecCode.h); flags select the returned dictionary contents.
        let flags = SecCSFlags(rawValue: 2)
        let infoStatus = SecCodeCopySigningInformation(staticCode, flags, &information)
        guard infoStatus == errSecSuccess, let dictionary = information as? [String: Any] else {
            return SigningIdentity(pid: pid, isValid: valid)
        }
        let flagsValue = dictionary[kSecCodeInfoFlags as String] as? UInt32 ?? 0
        // kSecCodeSignatureAdhoc == 0x2 (Security/SecCode.h)
        let isAdHoc = (flagsValue & 0x2) != 0
        let cdhash = (dictionary[kSecCodeInfoUnique as String] as? Data).map { data in
            data.map { String(format: "%02x", $0) }.joined()
        }
        return SigningIdentity(
            pid: pid,
            signingIdentifier: dictionary[kSecCodeInfoIdentifier as String] as? String,
            teamIdentifier: dictionary[kSecCodeInfoTeamIdentifier as String] as? String,
            cdhashHex: cdhash,
            isAdHoc: isAdHoc,
            isValid: valid,
            executablePath: nil,
            entitlements: entitlements(from: dictionary)
        )
    }

    private static func entitlements(from dictionary: [String: Any]) -> [String: JSONValue]? {
        guard let raw = dictionary[kSecCodeInfoEntitlementsDict as String] as? [String: Any] else { return nil }
        guard let data = try? JSONSerialization.data(withJSONObject: raw, options: [.fragmentsAllowed]) else {
            return nil
        }
        return (try? JSONDecoder().decode([String: JSONValue].self, from: data)) ?? nil
    }
}
