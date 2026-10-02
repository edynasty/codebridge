import Foundation

/// Canonical Host IPC locations and Phase 0 environment switches.
///
/// Socket path (frozen with the daemon): `~/Library/Application Support/CodeBridge/run/hostipc.sock`,
/// directory mode 0700. The environment override exists for development and for the Phase 0 smoke
/// runs; nothing else in the app derives paths from the bundle identifier.
public enum HostIPCPaths {
    public static let socketPathEnvironmentKey = "CODEBRIDGE_HOSTIPC_SOCKET"
    /// Client-side only: suppresses this process's own local daemon-peer signature check.
    ///
    /// It does **not** and cannot make `codebridged` accept an unverified peer: the daemon
    /// independently requires a live, signature-verified peer for `role = app` and answers
    /// `-32010 unauthorized_peer` otherwise. The only unsigned path is `role = diagnostics`
    /// (peer-UID only, `local_mcp` class, no `local_ui`, no computer/approval methods).
    public static let allowUnverifiedDaemonPeerEnvironmentKey = "CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER"
    public static let phase0DebugEnvironmentKey = "CODEBRIDGE_PHASE0_DEBUG"
    public static let phase0ProbePathEnvironmentKey = "CODEBRIDGE_PHASE0_PROBE"
    public static let expectedDaemonSigningIdentifierKey = "CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID"
    public static let expectedAppSigningIdentifierKey = "CODEBRIDGE_EXPECT_APP_SIGNING_ID"
    public static let expectedTeamIdentifierKey = "CODEBRIDGE_EXPECT_TEAM_ID"

    public static let daemonLaunchAgentLabel = "com.codebridge.daemon"
    public static let daemonLaunchAgentPlistName = "com.codebridge.daemon.plist"
    public static let defaultProbeExecutableName = "codebridge-probe"
    public static let appExecutableName = "CodeBridge"
    public static let daemonExecutableName = "codebridged"

    public static var homeDirectory: String {
        ProcessInfo.processInfo.environment["HOME"] ?? NSHomeDirectory()
    }

    public static var applicationSupportDirectory: String {
        homeDirectory + "/Library/Application Support/CodeBridge"
    }

    public static var runDirectory: String {
        applicationSupportDirectory + "/run"
    }

    public static var defaultSocketPath: String {
        runDirectory + "/hostipc.sock"
    }

    public static var socketPath: String {
        ProcessInfo.processInfo.environment[socketPathEnvironmentKey] ?? defaultSocketPath
    }

    public static var logsDirectory: String {
        homeDirectory + "/Library/Logs/CodeBridge"
    }

    /// Explicit local opt-in that lets this client continue when it cannot verify the daemon's
    /// signature. It only affects the client's own pre-check; it cannot produce an app session on a
    /// daemon that rejects unverified peers.
    public static var allowsUnverifiedPeer: Bool {
        ProcessInfo.processInfo.environment[allowUnverifiedDaemonPeerEnvironmentKey] == "1"
    }

    public static var phase0DebugEnabled: Bool {
        ProcessInfo.processInfo.environment[phase0DebugEnvironmentKey] == "1"
    }

    /// Environment overrides are for development; signed bundles seal their expected peer identity.
    public static func expectedDaemonSignature() -> ExpectedPeerSignature? {
        let environment = ProcessInfo.processInfo.environment
        let info = Bundle.main.infoDictionary ?? [:]
        let signedBundle = info["CodeBridgeBuildKind"] as? String == "signed"
            && info["CodeBridgeUnsignedDevBuild"] as? Bool == false
        guard let identifier = environment[expectedDaemonSigningIdentifierKey]
            ?? (signedBundle ? info["CodeBridgeExpectedDaemonSigningIdentifier"] as? String : nil),
            !identifier.isEmpty else { return nil }
        let team = (environment[expectedTeamIdentifierKey]
            ?? (signedBundle ? info["CodeBridgeExpectedTeamIdentifier"] as? String : nil)).flatMap { $0.isEmpty ? nil : $0 }
        return ExpectedPeerSignature(signingIdentifier: identifier, teamIdentifier: team)
    }

    public static func expectedProbePath() -> String? {
        let environment = ProcessInfo.processInfo.environment
        guard let path = environment[phase0ProbePathEnvironmentKey], !path.isEmpty else { return nil }
        return path
    }
}
