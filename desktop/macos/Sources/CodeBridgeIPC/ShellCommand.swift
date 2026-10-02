import Darwin
import Foundation

public struct ShellCommandResult {
    public var exitCode: Int32
    public var stdout: String
    public var stderr: String
    public var timedOut: Bool

    public var combinedFirstLine: String? {
        let text = stdout.isEmpty ? stderr : stdout
        return text
            .split(separator: "\n", omittingEmptySubsequences: true)
            .first
            .map { String($0).trimmingCharacters(in: .whitespaces) }
    }
}

final class ShellCommandCapture {
    var stdout = Data()
    var stderr = Data()
}

/// Minimal, timeout-bounded process runner.
///
/// Used for fixed, read-only diagnostics: host tool version checks (`git --version`, …) and
/// `launchctl print` inspections. Fixed argv only; no shell, no environment dump, no writes.
public enum ShellCommand {
    public static func resolve(_ name: String) -> String? {
        if name.hasPrefix("/") {
            return FileManager.default.isExecutableFile(atPath: name) ? name : nil
        }
        guard let path = ProcessInfo.processInfo.environment["PATH"] else { return nil }
        for directory in path.split(separator: ":") {
            let candidate = "\(directory)/\(name)"
            if FileManager.default.isExecutableFile(atPath: candidate) {
                return candidate
            }
        }
        return nil
    }

    public static func run(executable: String, arguments: [String], timeout: TimeInterval) -> ShellCommandResult {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = arguments

        let capture = ShellCommandCapture()
        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe
        process.standardInput = FileHandle.nullDevice

        let readers = DispatchGroup()
        readers.enter()
        DispatchQueue.global().async {
            capture.stdout = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
            readers.leave()
        }
        readers.enter()
        DispatchQueue.global().async {
            capture.stderr = stderrPipe.fileHandleForReading.readDataToEndOfFile()
            readers.leave()
        }

        do {
            try process.run()
        } catch {
            return ShellCommandResult(
                exitCode: 127,
                stdout: "",
                stderr: "spawn failed: \(error)",
                timedOut: false
            )
        }

        let deadline = Date().addingTimeInterval(timeout)
        while process.isRunning && Date() < deadline {
            usleep(50_000)
        }

        var timedOut = false
        if process.isRunning {
            timedOut = true
            process.terminate()
            usleep(200_000)
            if process.isRunning {
                kill(process.processIdentifier, SIGKILL)
            }
        }
        process.waitUntilExit()
        _ = readers.wait(timeout: .now() + 2)

        return ShellCommandResult(
            exitCode: process.terminationStatus,
            stdout: String(decoding: capture.stdout, as: UTF8.self),
            stderr: String(decoding: capture.stderr, as: UTF8.self),
            timedOut: timedOut
        )
    }
}
