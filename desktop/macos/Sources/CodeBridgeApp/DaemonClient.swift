import CodeBridgeComputerSpike
import CodeBridgeIPC
import Foundation

/// Synchronous Host IPC operations used by the menu bar. All calls happen on a background queue.
enum DaemonClient {
    struct Outcome {
        var ok: Bool
        var body: String
    }

    static func handshake(role: String, socketPath: String = HostIPCPaths.socketPath) -> Outcome {
        var configuration = HostIPCClient.Configuration(
            socketPath: socketPath,
            role: role,
            capabilities: role == HostIPCRole.diagnostics
                ? ["host", "native_host_smoke"]
                : ["local_ui", "computer"]
        )
        if role == HostIPCRole.diagnostics, configuration.expectedDaemon == nil {
            // Diagnostics is the documented unsigned path; it never reaches local_ui or computer verbs.
            configuration.allowUnverifiedPeer = true
        }

        var lines: [String] = [
            "role: \(configuration.role)",
            "socket: \(configuration.socketPath)",
            "expected daemon signature: "
                + (configuration.expectedDaemon.map { "\($0.signingIdentifier)/\($0.teamIdentifier ?? "-")" } ?? "not configured"),
            "client allows unverified daemon peer: \(configuration.allowUnverifiedPeer) "
                + "(client-side check only; the daemon enforces its own)",
        ]

        let client = HostIPCClient(configuration: configuration)
        do {
            let hello = try client.connect()
            if let credentials = client.peerCredentials {
                lines.append("peer: \(credentials)")
                lines.append("peer uid matches own uid: \(credentials.uid == getuid())")
            }
            lines.append("peer signature: \(client.peerVerification?.summary ?? "not checked")")
            lines.append("negotiated protocol: \(client.negotiatedProtocol.map { "\($0)" } ?? "unknown")")
            lines.append("daemon_version: \(hello.daemon_version)")
            lines.append("daemon role: \(hello.role ?? "unspecified")")
            lines.append("daemon capabilities: \((hello.capabilities ?? []).joined(separator: ","))")
            lines.append("handshake: accepted")
            client.close()
            return Outcome(ok: true, body: lines.joined(separator: "\n"))
        } catch let error as HostIPCError {
            lines.append("handshake failed: \(error)")
            if let code = error.rpcCode {
                lines.append("error code: \(code) (\(HostIPCErrorCode.name(for: code)))")
            }
            if role == HostIPCRole.app {
                lines.append(
                    "note: role=app needs a live signature-verified peer on the daemon side; without a signing "
                        + "identity use role=diagnostics for Probe Phase 0 runs"
                )
            }
            return Outcome(ok: false, body: lines.joined(separator: "\n"))
        } catch {
            lines.append("handshake failed: \(error)")
            return Outcome(ok: false, body: lines.joined(separator: "\n"))
        }
    }

    /// Runs the probe through the daemon so the probe process is a child of `codebridged`.
    static func daemonAttributedProbe(
        _ probe: String,
        socketPath: String = HostIPCPaths.socketPath,
        timeout: TimeInterval = 120
    ) -> Outcome {
        var configuration = HostIPCClient.Configuration.phase0Diagnostics(socketPath: socketPath)
        if configuration.expectedDaemon == nil {
            configuration.allowUnverifiedPeer = true
        }
        let client = HostIPCClient(configuration: configuration)
        var lines: [String] = []
        do {
            _ = try client.connect()
            lines.append("connected as \(configuration.role) (daemon-attributed child process run)")
            let result = try client.phase0Probe(probe, args: ["--json"], timeout: timeout)
            lines.append("probe exit code: \(result.exit_code), timed out: \(result.timed_out)")
            if !result.stderr.isEmpty {
                lines.append("probe stderr: \(String(result.stderr.prefix(2000)))")
            }
            if let path = ProbeReportStore.persist(result.stdout, probe: probe, attribution: "daemon-child") {
                lines.append("report saved: \(path)")
            }
            lines.append("")
            lines.append(summarize(result.stdout) ?? String(result.stdout.prefix(8000)))
            client.close()
            return Outcome(ok: result.exit_code == 0, body: lines.joined(separator: "\n"))
        } catch let error as HostIPCError {
            lines.append("probe via daemon failed: \(error)")
            if let code = error.rpcCode {
                lines.append("error code: \(code) (\(HostIPCErrorCode.name(for: code)))")
            }
            return Outcome(ok: false, body: lines.joined(separator: "\n"))
        } catch {
            lines.append("probe via daemon failed: \(error)")
            return Outcome(ok: false, body: lines.joined(separator: "\n"))
        }
    }

    static func summarize(_ json: String) -> String? {
        guard let data = json.data(using: .utf8),
              let value = try? JSONValue.decode(from: data)
        else { return nil }
        var lines: [String] = []
        if let probe = value["probe"]?.stringValue {
            lines.append("probe: \(probe)")
        }
        if let launch = value["launch"] {
            lines.append(
                "launch context: \(launch["launch_context"]?.stringValue ?? "unknown"), "
                    + "pid \(launch["pid"]?.intValue.map { String($0) } ?? "?"), "
                    + "ppid \(launch["parent_pid"]?.intValue.map { String($0) } ?? "?"), "
                    + "signed \(launch["signed"]?.boolValue.map { String($0) } ?? "?")"
            )
        }
        if let findings = value["findings"] {
            for key in ["observed", "untested", "blocked"] {
                guard let entries = findings[key]?.arrayValue else { continue }
                lines.append("\(key.uppercased()):")
                for entry in entries {
                    lines.append("  - \(entry.stringValue ?? "")")
                }
            }
        }
        return lines.isEmpty ? nil : lines.joined(separator: "\n")
    }
}

/// Stores raw probe JSON under `~/Library/Logs/CodeBridge/` so evidence keeps observed bytes.
enum ProbeReportStore {
    static func persist(_ json: String, probe: String, attribution: String) -> String? {
        let directory = URL(fileURLWithPath: HostIPCPaths.logsDirectory, isDirectory: true)
        let stamp = ProbeReportWriter.timestamp().replacingOccurrences(of: ":", with: "-")
        let url = directory.appendingPathComponent("probe-\(attribution)-\(probe)-\(stamp).json")
        do {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            try Data(json.utf8).write(to: url, options: .atomic)
            return url.path
        } catch {
            return nil
        }
    }
}
