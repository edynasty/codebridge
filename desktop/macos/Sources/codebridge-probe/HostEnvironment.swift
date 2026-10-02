import AppKit
import CodeBridgeComputerSpike
import CodeBridgeIPC
import CoreGraphics
import Foundation

/// Host environment and launch-identity sections of the probe report.
enum HostEnvironment {
    static func hostInfo() -> ProbeHostInfo {
        let sessionAvailable = CGSessionCopyCurrentDictionary() != nil
        return ProbeHostInfo(
            os_version: ProcessInfo.processInfo.operatingSystemVersionString,
            os_build: sysctlString("kern.osversion") ?? "unknown",
            architecture: architecture(),
            model_identifier: sysctlString("hw.model"),
            user_uid: getuid(),
            gui_session_available: sessionAvailable,
            launchd_domain: sessionAvailable ? "gui/\(getuid())" : nil
        )
    }

    static func launchIdentity() -> ProbeLaunchIdentity {
        let identity = ProcessIdentityReader.current()
        let signing = PeerIdentity.signingIdentity(pid: getpid())
        return ProbeLaunchIdentity(identity: identity, signing: signing)
    }

    static func signingExtra() -> SigningExtraSection {
        let parentPID = getppid()
        let parentSigning = PeerIdentity.signingIdentity(pid: parentPID)
        let teamConfigured = HostIPCPaths.expectedDaemonSignature() != nil
            || ProcessInfo.processInfo.environment[HostIPCPaths.expectedTeamIdentifierKey] != nil
        return SigningExtraSection(
            parent_signed: parentSigning?.signingIdentifier != nil,
            parent_signing_identifier: parentSigning?.signingIdentifier,
            parent_team_identifier: parentSigning?.teamIdentifier,
            parent_signature_error: parentSigning == nil
                ? "no code signature available for parent pid \(parentPID)"
                : nil,
            team_identifier_configured: teamConfigured,
            tcc_attribution_note: "TCC attribution requires comparing this report across launch contexts: "
                + "child of CodeBridge.app (app-attributed), child of codebridged (daemon-attributed) and "
                + "standalone. Grants are per code-signing identity; a process with no signature cannot be "
                + "granted or verified."
        )
    }

    static func architecture() -> String {
        #if arch(arm64)
        return "arm64"
        #elseif arch(x86_64)
        return "x86_64"
        #else
        return sysctlString("hw.machine") ?? "unknown"
        #endif
    }

    static func sysctlString(_ name: String) -> String? {
        var size = 0
        guard sysctlbyname(name, nil, &size, nil, 0) == 0, size > 0 else { return nil }
        var buffer = [CChar](repeating: 0, count: size)
        guard sysctlbyname(name, &buffer, &size, nil, 0) == 0 else { return nil }
        return String(cString: buffer)
    }
}
