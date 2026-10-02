import AppKit
import Darwin
import Foundation

/// Process identity used for launch-context and TCC-attribution evidence.
///
/// Attribution depends on *how* a process was launched, so the probe records its parent chain and
/// whether the parent is launchd (`ppid == 1`) rather than guessing from the executable path.
public struct ProcessIdentity: Codable, Equatable, Sendable, CustomStringConvertible {
    public var pid: Int32
    public var parentPID: Int32
    public var executablePath: String?
    public var parentExecutablePath: String?
    public var launchContext: String

    public init(
        pid: Int32,
        parentPID: Int32,
        executablePath: String?,
        parentExecutablePath: String?,
        launchContext: String
    ) {
        self.pid = pid
        self.parentPID = parentPID
        self.executablePath = executablePath
        self.parentExecutablePath = parentExecutablePath
        self.launchContext = launchContext
    }

    public var description: String {
        "pid=\(pid) ppid=\(parentPID) context=\(launchContext) path=\(executablePath ?? "unknown")"
    }
}

public enum ProcessIdentityReader {
    public static func current() -> ProcessIdentity {
        let pid = getpid()
        let parent = getppid()
        return ProcessIdentity(
            pid: pid,
            parentPID: parent,
            executablePath: executablePath(pid: pid),
            parentExecutablePath: executablePath(pid: parent),
            launchContext: launchContext(parentPID: parent, parentPath: executablePath(pid: parent))
        )
    }

    public static func executablePath(pid: pid_t) -> String? {
        var buffer = [CChar](repeating: 0, count: 4096)
        let resolved = proc_pidpath(pid, &buffer, UInt32(buffer.count))
        if resolved > 0 {
            return String(cString: buffer)
        }
        if let running = NSRunningApplication(processIdentifier: pid), let url = running.executableURL {
            return url.path
        }
        if pid == getpid() {
            return Bundle.main.executablePath ?? CommandLine.arguments.first
        }
        return nil
    }

    /// Parent pid of an arbitrary process (used to prove the daemon's parent is launchd).
    public static func parentPID(of pid: pid_t) -> pid_t? {
        var info = kinfo_proc()
        var size = MemoryLayout<kinfo_proc>.stride
        var mib: [Int32] = [CTL_KERN, KERN_PROC, KERN_PROC_PID, pid]
        let result = sysctl(&mib, UInt32(mib.count), &info, &size, nil, 0)
        guard result == 0, size > 0 else { return nil }
        return info.kp_eproc.e_ppid
    }

    public static func launchContext(parentPID: pid_t, parentPath: String?) -> String {
        if parentPID == 1 { return "launchd" }
        if let path = parentPath {
            let name = (path as NSString).lastPathComponent
            if name == "CodeBridge" { return "codebridge_app_child" }
            if name == "codebridged" { return "codebridged_child" }
            if ["bash", "zsh", "sh", "fish", "login", "sudo", "env"].contains(name) {
                return "shell_child:\(name)"
            }
            return "process_child:\(name)"
        }
        return "unknown_parent:\(parentPID)"
    }
}
