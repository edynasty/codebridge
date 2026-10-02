import CodeBridgeComputerSpike
import CodeBridgeIPC
import Foundation

/// Native host probe: availability and version of the tools the daemon is expected to reach.
///
/// Fixed, read-only argv per tool. The authoritative "works from `codebridged`" evidence is the
/// daemon's own `host.native_host_smoke`; this probe reports the same checks from its own process
/// tree so the launch context can be compared.
enum HostToolsProbe {
    struct Check {
        let name: String
        let executable: String
        let arguments: [String]
    }

    static let checks: [Check] = [
        Check(name: "shell", executable: "/bin/zsh", arguments: ["--version"]),
        Check(name: "git", executable: "git", arguments: ["--version"]),
        Check(name: "docker", executable: "docker", arguments: ["--version"]),
        Check(name: "ssh", executable: "ssh", arguments: ["-V"]),
        Check(name: "kubectl", executable: "kubectl", arguments: ["version", "--client"]),
    ]

    static func run(only: [String]?) -> HostToolsSection {
        let wanted = checks.filter { check in
            guard let only = only, !only.isEmpty else { return true }
            return only.contains(check.name)
        }

        var reports: [HostToolProbeReport] = []
        for check in wanted {
            guard let resolved = ShellCommand.resolve(check.executable) else {
                reports.append(
                    HostToolProbeReport(
                        name: check.name,
                        resolved_path: nil,
                        available: false,
                        version: nil,
                        detail: "not found on PATH or not executable"
                    )
                )
                continue
            }
            let result = ShellCommand.run(executable: resolved, arguments: check.arguments, timeout: 5)
            let version = result.timedOut ? nil : result.combinedFirstLine
            var detail: String?
            if result.timedOut {
                detail = "timed out after 5s"
            } else if result.exitCode != 0 {
                detail = "exit status \(result.exitCode)"
            }
            reports.append(
                HostToolProbeReport(
                    name: check.name,
                    resolved_path: resolved,
                    available: true,
                    version: version.map { String($0.prefix(200)) },
                    detail: detail
                )
            )
        }

        return HostToolsSection(
            tools: reports,
            launch_context: ProcessIdentityReader.current().launchContext,
            note: "read-only version checks with fixed argv; the environment is never dumped"
        )
    }
}
