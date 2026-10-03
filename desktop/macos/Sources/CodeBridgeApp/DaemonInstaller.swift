import CodeBridgeIPC
import Foundation
import Security

/// Production F2 topology (2026-10-03 architecture amendment): the daemon is a separately signed
/// per-user LaunchAgent installed physically OUTSIDE CodeBridge.app. This controller implements
/// the install/update/uninstall transaction and launchd lifecycle for that external daemon.
///
/// Layout:
///   ~/Library/Application Support/CodeBridge/bin/codebridged   (stable production path)
///   ~/Library/LaunchAgents/com.codebridge.daemon.plist         (user LaunchAgent)
///
/// The app bundle carries `Contents/Resources/DaemonPayload/codebridged` purely as a source
/// artifact for installation; launchd only ever executes the installed external copy.
///
/// Security invariants:
/// - payload and installed binary are verified via SecStaticCode/SecCodeCheckValidity
///   (identifier com.codebridge.daemon, expected Team, non-ad-hoc, Apple-anchored DR);
/// - install is staged and atomically swapped; failure rolls back;
/// - launchctl is invoked with a strict argv (no shell), timeout and structured errors.
enum DaemonInstallerError: Error, CustomStringConvertible {
    case payloadMissing(String)
    case signatureVerificationFailed(String)
    case stagingFailed(String)
    case plistWriteFailed(String)
    case launchctlFailed(command: String, exitCode: Int32, stderr: String)
    case daemonNotRunning
    case rollback(String)
    case externalLocationPolicyFailed(String)

    var description: String {
        switch self {
        case .payloadMissing(let p): return "daemon payload missing: \(p)"
        case .signatureVerificationFailed(let d): return "signature verification failed: \(d)"
        case .stagingFailed(let d): return "staging failed: \(d)"
        case .plistWriteFailed(let d): return "LaunchAgent plist write failed: \(d)"
        case .launchctlFailed(let c, let e, let s): return "launchctl \(c) failed (exit \(e)): \(s)"
        case .daemonNotRunning: return "daemon is not running after bootstrap"
        case .rollback(let d): return "install rolled back: \(d)"
        case .externalLocationPolicyFailed(let d): return "external-location policy failed: \(d)"
        }
    }
}

final class DaemonInstaller {
    struct InstalledDaemon {
        let url: URL
        let cdhash: String?
        let teamIdentifier: String?
    }

    /// The daemon's signing identifier; matches the sealed Info.plist expectation.
    static let daemonSigningIdentifier = "com.codebridge.daemon"

    let supportDirectory: URL
    let binDirectory: URL
    let daemonURL: URL
    let stagingDirectory: URL
    let plistURL: URL
    let label: String

    init(supportDirectory: URL = FileManager.default
            .urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("CodeBridge", isDirectory: true)) {
        self.supportDirectory = supportDirectory
        self.binDirectory = supportDirectory.appendingPathComponent("bin", isDirectory: true)
        self.daemonURL = binDirectory.appendingPathComponent(HostIPCPaths.daemonExecutableName)
        self.stagingDirectory = supportDirectory.appendingPathComponent(".install-staging", isDirectory: true)
        let home = FileManager.default.homeDirectoryForCurrentUser
        self.plistURL = home
            .appendingPathComponent("Library/LaunchAgents", isDirectory: true)
            .appendingPathComponent(HostIPCPaths.daemonLaunchAgentPlistName)
        self.label = HostIPCPaths.daemonLaunchAgentLabel
    }

    /// Shared production installer (stable user-level paths).
    static let shared = DaemonInstaller()

    // MARK: - Paths

    var payloadURL: URL {
        Bundle.main.bundleURL.appendingPathComponent("Contents/Resources/DaemonPayload/\(HostIPCPaths.daemonExecutableName)")
    }

    var isInstalled: Bool {
        FileManager.default.fileExists(atPath: daemonURL.path)
    }

    /// The production daemon must live outside the app bundle (F2 topology invariant).
    func verifyExternalLocation(_ url: URL) throws {
        let bundlePath = Bundle.main.bundlePath
        let standardized = url.standardizedFileURL.path
        guard !standardized.hasPrefix(bundlePath + "/") else {
            throw DaemonInstallerError.externalLocationPolicyFailed(
                "daemon path \(standardized) is inside the app bundle"
            )
        }
    }

    // MARK: - Signature verification (Security framework, not codesign CLI parsing)

    /// Verifies a static code at `url` against the daemon requirement: identifier, expected team
    /// (from the app's own Info.plist, not hard-coded), Apple-anchored DR, non-ad-hoc.
    func verifySignature(at url: URL) throws -> InstalledDaemon {
        var staticCode: SecStaticCode?
        let status = SecStaticCodeCreateWithPath(url as CFURL, [], &staticCode)
        guard status == errSecSuccess, let code = staticCode else {
            throw DaemonInstallerError.signatureVerificationFailed(
                "SecStaticCodeCreateWithPath osStatus \(status) for \(url.path)"
            )
        }

        let expectedTeam = (Bundle.main.object(forInfoDictionaryKey: "CodeBridgeExpectedTeamIdentifier") as? String) ?? ""
        guard let requirement = PeerRequirement.make(
            identifier: DaemonInstaller.daemonSigningIdentifier, teamIdentifier: expectedTeam
        ) else {
            throw DaemonInstallerError.signatureVerificationFailed("cannot build peer requirement")
        }
        let check = SecStaticCodeCheckValidity(code, [], requirement)
        guard check == errSecSuccess else {
            throw DaemonInstallerError.signatureVerificationFailed(
                "SecCodeCheckValidity osStatus \(check) for \(url.path)"
            )
        }

        var info: CFDictionary?
        guard SecCodeCopySigningInformation(code, SecCSFlags(rawValue: kSecCSSigningInformation), &info) == errSecSuccess,
              let signing = info as? [String: Any] else {
            throw DaemonInstallerError.signatureVerificationFailed("SecCodeCopySigningInformation failed")
        }
        guard (signing[kSecCodeInfoIdentifier as String] as? String) == DaemonInstaller.daemonSigningIdentifier else {
            throw DaemonInstallerError.signatureVerificationFailed("identifier mismatch")
        }
        let team = signing[kSecCodeInfoTeamIdentifier as String] as? String
        guard let team, !team.isEmpty else {
            throw DaemonInstallerError.signatureVerificationFailed("missing team identifier (ad-hoc?)")
        }
        let cdhash = (signing[kSecCodeInfoUnique as String] as? Data)?
            .map { String(format: "%02x", $0) }.joined()
        return InstalledDaemon(url: url, cdhash: cdhash, teamIdentifier: team)
    }

    // MARK: - Install transaction

    /// Installs (or updates) the external daemon and bootstraps the LaunchAgent.
    /// Any failure rolls back to the previous binary or leaves the daemon uninstalled.
    func install() throws -> InstalledDaemon {
        let fm = FileManager.default
        guard fm.fileExists(atPath: payloadURL.path) else {
            throw DaemonInstallerError.payloadMissing(payloadURL.path)
        }
        // Verify the source payload before copying.
        _ = try verifySignature(at: payloadURL)

        // 1. directories
        try fm.createDirectory(at: binDirectory, withIntermediateDirectories: true)
        try fm.createDirectory(at: stagingDirectory, withIntermediateDirectories: true)
        let hadPrevious = fm.fileExists(atPath: daemonURL.path)

        // 2. stage the payload copy on the same filesystem
        let stagingBinary = stagingDirectory.appendingPathComponent(HostIPCPaths.daemonExecutableName)
        try? fm.removeItem(at: stagingBinary)
        try fm.copyItem(at: payloadURL, to: stagingBinary)
        try fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: stagingBinary.path)

        // 3. verify staged signature
        let staged = try verifySignature(at: stagingBinary)

        // 4. atomic replace into the stable path
        try fm.removeItem(at: daemonURL) // no-op when absent
        try fm.moveItem(at: stagingBinary, to: daemonURL)
        do {
            try writeLaunchAgentPlist()
        } catch {
            // rollback binary
            try? fm.removeItem(at: daemonURL)
            throw error
        }

        // 5. launchd bootstrap (idempotent: bootout stale job first)
        do {
            try launchctl(["bootout", "gui/\(getuid())/\(label)"])
        } catch DaemonInstallerError.launchctlFailed(_, let code, _) where code == 5 { /* not loaded */ }
        try launchctl(["bootstrap", "gui/\(String(getuid()))", plistURL.path])
        try launchctl(["kickstart", "-k", "gui/\(getuid())/\(label)"])

        // 6. verify the daemon actually runs under launchd
        let info = try DaemonLaunchctl.inspect(label: label)
        guard let pid = info.pid, pid > 0 else {
            if !hadPrevious {
                try? launchctl(["bootout", "gui/\(getuid())/\(label)"])
                try? fm.removeItem(at: plistURL)
                try? fm.removeItem(at: daemonURL)
            }
            throw DaemonInstallerError.daemonNotRunning
        }
        return staged
    }

    /// Uninstalls the external daemon: bootout, remove plist and binary, keep runtime data.
    func uninstall() throws {
        let fm = FileManager.default
        _ = try? launchctl(["bootout", "gui/\(getuid())/\(label)"])
        if fm.fileExists(atPath: plistURL.path) { try fm.removeItem(at: plistURL) }
        if fm.fileExists(atPath: daemonURL.path) { try fm.removeItem(at: daemonURL) }
        _ = try? fm.removeItem(at: stagingDirectory)
    }

    // MARK: - LaunchAgent plist

    func writeLaunchAgentPlist() throws {
        let fm = FileManager.default
        let agentsDirectory = plistURL.deletingLastPathComponent()
        try fm.createDirectory(at: agentsDirectory, withIntermediateDirectories: true)

        let programArguments = [daemonURL.path, "run"]
        let plist: [String: Any] = [
            "Label": label,
            "ProgramArguments": programArguments,
            "RunAtLoad": true,
            "KeepAlive": ["SuccessfulExit": false],
            "ThrottleInterval": 5,
            "StandardOutPath": supportDirectory.appendingPathComponent("logs/daemon.stdout.log").path,
            "StandardErrorPath": supportDirectory.appendingPathComponent("logs/daemon.stderr.log").path,
        ]
        let data = try PropertyListSerialization.data(fromPropertyList: plist, format: .xml, options: 0)
        let stagingPlist = stagingDirectory.appendingPathComponent(label + ".plist")
        try data.write(to: stagingPlist)
        guard (try? stagedPlistIsValid(stagingPlist)) == true else {
            throw DaemonInstallerError.plistWriteFailed("generated plist failed plutil -lint")
        }
        if fm.fileExists(atPath: plistURL.path) { try fm.removeItem(at: plistURL) }
        try fm.moveItem(at: stagingPlist, to: plistURL)
        try fm.createDirectory(
            at: supportDirectory.appendingPathComponent("logs", isDirectory: true),
            withIntermediateDirectories: true
        )
    }

    private func stagedPlistIsValid(_ url: URL) throws -> Bool {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/plutil")
        process.arguments = ["-lint", url.path]
        try process.run()
        process.waitUntilExit()
        return process.terminationStatus == 0
    }

    // MARK: - launchctl encapsulation

    /// Runs launchctl with a strict argv (never a shell string), with a timeout, and maps
    /// failures to structured errors.
    @discardableResult
    func launchctl(_ arguments: [String], timeout: TimeInterval = 15) throws -> String {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        process.arguments = arguments
        let out = Pipe()
        let err = Pipe()
        process.standardOutput = out
        process.standardError = err
        try process.run()

        let queue = DispatchQueue(label: "cb.launchctl")
        var timedOut = false
        queue.asyncAfter(deadline: .now() + timeout) { [weak process] in
            if process?.isRunning == true { timedOut = true; process?.terminate() }
        }
        process.waitUntilExit()
        let stderrData = err.fileHandleForReading.readDataToEndOfFile()
        let stderrText = String(data: stderrData, encoding: .utf8) ?? ""
        guard !timedOut, process.terminationStatus == 0 else {
            throw DaemonInstallerError.launchctlFailed(
                command: arguments.joined(separator: " "),
                exitCode: process.terminationStatus,
                stderr: timedOut ? "timed out after \(timeout)s" : stderrText
            )
        }
        return String(data: out.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
    }
}

/// Read-only launchd inspection for the external daemon job.
enum DaemonLaunchctl {
    struct JobSnapshot {
        let pid: Int32?
        let state: String?
        let program: String?
    }

    static func inspect(label: String) throws -> JobSnapshot {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        process.arguments = ["print", "gui/\(getuid())/\(label)"]
        let out = Pipe()
        process.standardOutput = out
        process.standardError = Pipe()
        try process.run()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else {
            throw DaemonInstallerError.launchctlFailed(
                command: "print", exitCode: process.terminationStatus,
                stderr: "job not found"
            )
        }
        let text = String(data: out.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
        var pid: Int32?
        var state: String?
        var program: String?
        for line in text.split(separator: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if trimmed.hasPrefix("pid ="), let value = Int32(trimmed.split(separator: "=")[1].trimmingCharacters(in: .whitespaces)) {
                pid = value
            }
            if trimmed.hasPrefix("state =") {
                state = trimmed.split(separator: "=", maxSplits: 1)[1]
                    .trimmingCharacters(in: .whitespaces)
            }
            if trimmed.hasPrefix("program =") {
                program = trimmed.split(separator: "=", maxSplits: 1)[1]
                    .trimmingCharacters(in: .whitespaces)
            }
        }
        return JobSnapshot(pid: pid, state: state, program: program)
    }
}
