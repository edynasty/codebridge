import CodeBridgeIPC
import Foundation

/// Runs the bundled `codebridge-probe` as a **transient child of CodeBridge.app**.
///
/// This is the app-attributed half of the TCC attribution spike: comparing this run with the
/// daemon-attributed run (`DaemonClient.daemonAttributedProbe`) shows whether a child inherits the
/// app's Screen Recording / Accessibility / Input Monitoring grants. The durable daemon is never
/// spawned from here — only launchd starts it (`SMAppService`).
enum ProbeChildRunner {
    static func bundledProbePath() -> String? {
        if let override = HostIPCPaths.expectedProbePath() {
            return FileManager.default.isExecutableFile(atPath: override) ? override : nil
        }
        let path = Bundle.main.bundlePath + "/Contents/MacOS/" + HostIPCPaths.defaultProbeExecutableName
        return FileManager.default.isExecutableFile(atPath: path) ? path : nil
    }

    static func run(probe: String, timeout: TimeInterval = 120) -> DaemonClient.Outcome {
        guard let executable = bundledProbePath() else {
            return DaemonClient.Outcome(
                ok: false,
                body: "bundled probe not found at Contents/MacOS/\(HostIPCPaths.defaultProbeExecutableName) "
                    + "and \(HostIPCPaths.phase0ProbePathEnvironmentKey) is unset"
            )
        }
        let result = ShellCommand.run(executable: executable, arguments: [probe, "--json"], timeout: timeout)
        var lines: [String] = [
            "probe binary: \(executable)",
            "probe pid tree: probe is a child of this CodeBridge.app process (pid \(getpid()))",
            "exit code: \(result.exitCode), timed out: \(result.timedOut)",
        ]
        if !result.stderr.isEmpty {
            lines.append("probe stderr: \(String(result.stderr.prefix(2000)))")
        }
        if let path = ProbeReportStore.persist(result.stdout, probe: probe, attribution: "app-child") {
            lines.append("report saved: \(path)")
        }
        lines.append("")
        lines.append(DaemonClient.summarize(result.stdout) ?? String(result.stdout.prefix(8000)))
        return DaemonClient.Outcome(ok: result.exitCode == 0, body: lines.joined(separator: "\n"))
    }
}
