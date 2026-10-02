import CodeBridgeComputerSpike
import CodeBridgeIPC
import Darwin
import Foundation

final class RootProbeBox {
    var report: ProtectedRootProbeReport?
}

/// Protected-root (Files and Folders) probe.
///
/// Read-only: `stat` plus a directory-entry **count** from `opendir`/`readdir`. No file name is
/// reported, nothing is created, modified or deleted.
///
/// Each root has a five-second report deadline. A timeout is not evidence of a TCC denial:
/// native filesystem access can remain blocked on the detached thread until this short-lived
/// probe process exits. This is bounded probe reporting, not a production ProjectRegistration API.
enum FilesFoldersProbe {
    static let perRootTimeout: TimeInterval = 5

    static func run(rootNames: [String], timeout: TimeInterval = perRootTimeout) -> FilesFoldersProbeReport {
        let home = HostIPCPaths.homeDirectory
        let roots = rootNames.map { name -> ProtectedRootProbeReport in
            let path = name.hasPrefix("/") ? name : home + "/" + name
            return probe(path: path, timeout: timeout)
        }
        return FilesFoldersProbeReport(
            roots: roots,
            note: "read-only opendir on the root plus entry counting, bounded at \(Int(timeout))s per root; "
                + "file names are neither reported nor persisted; nothing is created, modified or deleted"
        )
    }

    static func probe(path: String, timeout: TimeInterval = perRootTimeout) -> ProtectedRootProbeReport {
        let box = RootProbeBox()
        let semaphore = DispatchSemaphore(value: 0)
        let started = Date()
        Thread.detachNewThread {
            box.report = probeUnbounded(path: path)
            semaphore.signal()
        }
        if semaphore.wait(timeout: .now() + timeout) == .timedOut {
            return ProtectedRootProbeReport(
                path: path,
                exists: true,
                readable: false,
                errno: nil,
                entry_count: nil,
                blocked_seconds: Date().timeIntervalSince(started),
                note: "deadline exceeded; existence and access are unresolved, not a proven TCC denial; "
                    + "native access may remain blocked until the short-lived probe process exits"
            )
        }
        return box.report ?? ProtectedRootProbeReport(
            path: path,
            exists: false,
            readable: false,
            errno: nil,
            entry_count: nil,
            blocked_seconds: nil,
            note: "probe returned no result"
        )
    }

    private static func probeUnbounded(path: String) -> ProtectedRootProbeReport {
        var isDirectory: ObjCBool = false
        let exists = FileManager.default.fileExists(atPath: path, isDirectory: &isDirectory)
        guard exists else {
            return ProtectedRootProbeReport(path: path, exists: false, readable: false, errno: nil, entry_count: nil)
        }

        guard let stream = opendir(path) else {
            let code = errno
            return ProtectedRootProbeReport(path: path, exists: true, readable: false, errno: code, entry_count: nil)
        }
        var entryCount = 0
        while let entry = readdir(stream) {
            let name = withUnsafeBytes(of: entry.pointee.d_name) { raw -> String in
                guard let base = raw.baseAddress else { return "" }
                return String(cString: base.assumingMemoryBound(to: CChar.self))
            }
            if name == "." || name == ".." { continue }
            entryCount += 1
        }
        closedir(stream)
        return ProtectedRootProbeReport(path: path, exists: true, readable: true, errno: nil, entry_count: entryCount)
    }
}
