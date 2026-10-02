import CodeBridgeIPC
import Foundation
import ServiceManagement

/// launchd's view of the daemon job. The app never starts the daemon itself: `SMAppService` hands
/// the job to launchd, and this reads back what launchd did (architecture §5.3).
struct DaemonJobInfo: Codable {
    var status: String
    var available: Bool
    var running: Bool
    var pid: Int32?
    var parentPID: Int32?
    var state: String?
    var program: String?
    var detail: String
}

/// Registers the bundled `codebridged` LaunchAgent through `SMAppService`.
final class LaunchAgentController {
    private let plistName: String
    private let label: String
    private let service: SMAppService

    init(plistName: String = HostIPCPaths.daemonLaunchAgentPlistName,
         label: String = HostIPCPaths.daemonLaunchAgentLabel) {
        self.plistName = plistName
        self.label = label
        self.service = SMAppService.agent(plistName: plistName)
    }

    var status: SMAppService.Status { service.status }

    var statusDescription: String {
        switch service.status {
        case .notRegistered: return "not registered"
        case .enabled: return "enabled"
        case .requiresApproval: return "requires approval (System Settings → General → Login Items)"
        case .notFound: return "plist not found in the bundle"
        @unknown default: return "unknown"
        }
    }

    var bundledPlistPath: String {
        Bundle.main.bundlePath + "/Contents/Library/LaunchAgents/" + plistName
    }

    var bundledDaemonPath: String {
        Bundle.main.bundlePath + "/Contents/MacOS/" + HostIPCPaths.daemonExecutableName
    }

    var plistIsPresent: Bool {
        FileManager.default.fileExists(atPath: bundledPlistPath)
    }

    var daemonBinaryIsPresent: Bool {
        FileManager.default.fileExists(atPath: bundledDaemonPath)
    }

    func register() throws {
        try Phase0BAcceptance.requireSignedBundle()
        try service.register()
    }

    func unregister() throws {
        try service.unregister()
    }

    /// Reads `launchctl print gui/<uid>/<label>`; read-only inspection, no spawn of the daemon.
    func daemonJobInfo() -> DaemonJobInfo {
        let result = ShellCommand.run(
            executable: "/bin/launchctl",
            arguments: ["print", "gui/\(getuid())/\(label)"],
            timeout: 5
        )
        var pid: Int32?
        var state: String?
        var program: String?
        for line in result.stdout.split(separator: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if trimmed.hasPrefix("pid = ") {
                pid = Int32(trimmed.dropFirst("pid = ".count).trimmingCharacters(in: .whitespaces))
            } else if trimmed.hasPrefix("state = ") {
                state = String(trimmed.dropFirst("state = ".count))
            } else if trimmed.hasPrefix("program = ") {
                program = String(trimmed.dropFirst("program = ".count))
            }
        }
        let parentPID = pid.flatMap { ProcessIdentityReader.parentPID(of: $0) }
        var detail = "launchctl exit \(result.exitCode)"
        if result.exitCode != 0 {
            detail += "; the job is not loaded in gui/\(getuid())"
        }
        if let pid = pid, parentPID == 1 {
            detail += "; daemon pid \(pid) has ppid 1 (launchd owns it, not the app)"
        } else if let pid = pid {
            detail += "; daemon pid \(pid) parent \(parentPID.map { String($0) } ?? "unknown")"
        }
        return DaemonJobInfo(
            status: statusDescription,
            available: result.exitCode == 0,
            running: pid != nil,
            pid: pid,
            parentPID: parentPID,
            state: state,
            program: program,
            detail: detail
        )
    }
}
