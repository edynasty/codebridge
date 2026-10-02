import CodeBridgeComputerSpike
import CodeBridgeIPC
import Foundation

/// Host IPC handshake smoke: connects to a running `codebridged` as a given role and reports exactly
/// what the frozen wire did. It never calls a computer, approval or runtime method.
enum HostIPCHandshakeProbe {
    struct Result: Codable, Sendable {
        var probe: String
        var socket_path: String
        var role: String
        var expected_daemon_signature: String?
        var client_allows_unverified_daemon_peer: Bool
        var connected: Bool
        var negotiated_protocol: String?
        var daemon_version: String?
        var daemon_capabilities: [String]?
        var daemon_role: String?
        var peer_pid: Int32?
        var peer_uid: uid_t?
        var peer_uid_matches: Bool?
        var peer_signature: String?
        var error: String?
        var error_code: Int?
        var observed: [String]
        var untested: [String]
        var blocked: [String]
    }

    static func run(role: String, pretty: Bool, outputPath: String?) throws -> Int32 {
        let configuration: HostIPCClient.Configuration = role == HostIPCRole.diagnostics
            ? HostIPCClient.Configuration.phase0Diagnostics()
            : HostIPCClient.Configuration(role: role)
        let client = HostIPCClient(configuration: configuration)

        var observed: [String] = []
        var untested: [String] = []
        var blocked: [String] = []
        var errorText: String?
        var errorCode: Int?
        var connected = false
        var hello: HostHelloResult?
        var credentials: PeerCredentials?

        do {
            hello = try client.connect()
            connected = true
            credentials = client.peerCredentials
            observed.append(
                "handshake accepted; negotiated protocol "
                    + "\(client.negotiatedProtocol.map { "\($0)" } ?? "unknown")"
            )
            if let credentials = credentials {
                observed.append("peer credentials: \(credentials)")
            }
            observed.append("peer signature: \(client.peerVerification?.summary ?? "not checked")")
            if let capabilities = hello?.capabilities {
                observed.append("daemon capabilities: \(capabilities.joined(separator: ","))")
            }
        } catch let hostError as HostIPCError {
            errorText = hostError.description
            errorCode = hostError.rpcCode
            if let code = errorCode {
                observed.append(
                    "daemon answered RPC error \(code) (\(HostIPCErrorCode.name(for: code)))"
                )
            } else {
                observed.append("transport-level failure before any daemon response")
            }
            credentials = client.peerCredentials
        } catch {
            errorText = "\(error)"
        }

        untested.append(
            "OS-capable Host IPC services: `computer` (daemon → app) and `approval.present` / `approval.cancel` "
                + "(daemon → app) plus `approval.decision` (app → daemon) are declared in the schema for Phase 1; "
                + "Phase 0 exercises none of them, so no session, frame or approval path is verified here"
        )
        if role == HostIPCRole.app && !connected {
            blocked.append(
                "role=app requires a live signature-verified peer against the daemon's configured team+bundle; "
                    + "this host has no signing identity, so an app session cannot be established yet"
            )
        }
        if !configuration.allowUnverifiedPeer, configuration.expectedDaemon == nil {
            untested.append(
                "daemon signature verification stayed disabled because no expected daemon signing identity is "
                    + "configured on this host"
            )
        }
        if !connected, let errorText = errorText {
            blocked.append("handshake did not complete: \(errorText)")
        }

        let result = Result(
            probe: "host-ipc-handshake",
            socket_path: configuration.socketPath,
            role: configuration.role,
            expected_daemon_signature: configuration.expectedDaemon
                .map { "\($0.signingIdentifier)/\($0.teamIdentifier ?? "-")" },
            client_allows_unverified_daemon_peer: configuration.allowUnverifiedPeer,
            connected: connected,
            negotiated_protocol: client.negotiatedProtocol.map { "\($0)" },
            daemon_version: hello?.daemon_version,
            daemon_capabilities: hello?.capabilities,
            daemon_role: hello?.role,
            peer_pid: credentials?.pid,
            peer_uid: credentials?.uid,
            peer_uid_matches: credentials.map { $0.uid == getuid() },
            peer_signature: client.peerVerification?.summary,
            error: errorText,
            error_code: errorCode,
            observed: observed,
            untested: untested,
            blocked: blocked
        )
        try ProbeReportWriter.emit(result, pretty: pretty, outputPath: outputPath)
        return connected ? 0 : 1
    }
}
