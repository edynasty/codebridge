import Foundation

/// Host IPC method and role names (frozen with `codebridged` 2026-10-02).
public enum HostIPCMethod {
    public static let hello = "host.hello"
    public static let health = "host.health"
    public static let prepareRestart = "host.prepare_restart"
    public static let phase0Probe = "host.phase0_probe"
    public static let nativeHostSmoke = "host.native_host_smoke"
}

public enum HostIPCRole {
    /// The signed CodeBridge.app. Daemon → app for the OS-capable services (`computer`,
    /// `approval.present` / `approval.cancel`, `notify.post`); app → daemon for decisions
    /// (`approval.decision`) and `host.*`. Never verified without an Apple-anchored requirement.
    public static let app = "app"
    /// Local Phase 0 tooling without a signing identity; never `local_ui` and never OS-capable
    /// services (the daemon answers `role_forbidden` for those).
    public static let diagnostics = "diagnostics"
    /// Harness subprocesses launched by the daemon; same restrictions as diagnostics, and never an
    /// OS-capable role.
    public static let harness = "harness"
}

public struct HostHelloParams: Codable, Sendable {
    public var `protocol`: ProtocolVersion
    public var app_version: String
    public var role: String
    public var capabilities: [String]
    public var bundle_id: String
    public var pid: Int32

    public init(
        protocol: ProtocolVersion,
        appVersion: String,
        role: String,
        capabilities: [String],
        bundleIdentifier: String,
        pid: Int32
    ) {
        self.`protocol` = `protocol`
        self.app_version = appVersion
        self.role = role
        self.capabilities = capabilities
        self.bundle_id = bundleIdentifier
        self.pid = pid
    }
}

public struct HostHelloResult: Codable, Sendable {
    public var accepted: Bool
    public var `protocol`: ProtocolVersion
    public var daemon_version: String
    public var capabilities: [String]?
    public var session_id: String?
    public var role: String?
    public var reason: String?
}

public struct Phase0ProbeParams: Codable, Sendable {
    public var probe: String
    public var args: [String]
    public var timeout_ms: Int

    public init(probe: String, args: [String], timeoutMilliseconds: Int) {
        self.probe = probe
        self.args = args
        self.timeout_ms = timeoutMilliseconds
    }
}

public struct Phase0ProbeResult: Codable, Sendable {
    public var exit_code: Int32
    public var stdout: String
    public var stderr: String
    public var timed_out: Bool

    /// Parses the probe's stdout as a `Phase0ProbeReport` when the probe printed one.
    public func report() -> JSONValue? {
        guard let data = stdout.data(using: .utf8) else { return nil }
        return try? JSONValue.decode(from: data)
    }
}

public struct NativeHostSmokeParams: Codable, Sendable {
    public var only: [String]?

    public init(only: [String]? = nil) {
        self.only = only
    }
}

public enum HostIPCError: Error, CustomStringConvertible, Equatable {
    case transport(String)
    case notConnected
    case peerUIDMismatch(expected: uid_t, actual: uid_t)
    case unverifiedPeer(String)
    case protocolRejected(String)
    case handshakeRejected(String)
    case rpc(code: Int, name: String, message: String)
    case timedOut(method: String)
    case malformedResponse(String)
    case unexpectedBinaryFrame

    public var description: String {
        switch self {
        case .transport(let detail):
            return "transport: \(detail)"
        case .notConnected:
            return "not connected to codebridged"
        case .peerUIDMismatch(let expected, let actual):
            return "peer UID mismatch (expected \(expected), got \(actual))"
        case .unverifiedPeer(let detail):
            return "unverified peer: \(detail)"
        case .protocolRejected(let detail):
            return "protocol rejected: \(detail)"
        case .handshakeRejected(let detail):
            return "handshake rejected: \(detail)"
        case .rpc(let code, let name, let message):
            return "daemon error \(code) (\(name)): \(message)"
        case .timedOut(let method):
            return "\(method) timed out"
        case .malformedResponse(let detail):
            return "malformed response: \(detail)"
        case .unexpectedBinaryFrame:
            return "unexpected binary attachment frame in Phase 0"
        }
    }

    public var rpcCode: Int? {
        if case .rpc(let code, _, _) = self { return code }
        return nil
    }
}

/// Synchronous Host IPC client (framing + JSON-RPC 2.0 `host` service).
///
/// Usage: `connect()` performs the socket connect, peer checks and `host.hello` handshake in one
/// step, exactly as the frozen contract requires.
public final class HostIPCClient {
    public struct Configuration {
        public var socketPath: String
        public var role: String
        public var appVersion: String
        public var bundleIdentifier: String
        public var capabilities: [String]
        public var expectedDaemon: ExpectedPeerSignature?
        public var allowUnverifiedPeer: Bool
        public var timeout: TimeInterval

        public init(
            socketPath: String = HostIPCPaths.socketPath,
            role: String = HostIPCRole.app,
            appVersion: String = HostIPCClient.defaultAppVersion,
            bundleIdentifier: String = HostIPCClient.defaultBundleIdentifier,
            capabilities: [String] = ["local_ui", "computer"],
            expectedDaemon: ExpectedPeerSignature? = HostIPCPaths.expectedDaemonSignature(),
            allowUnverifiedPeer: Bool = HostIPCPaths.allowsUnverifiedPeer,
            timeout: TimeInterval = 5
        ) {
            self.socketPath = socketPath
            self.role = role
            self.appVersion = appVersion
            self.bundleIdentifier = bundleIdentifier
            self.capabilities = capabilities
            self.expectedDaemon = expectedDaemon
            self.allowUnverifiedPeer = allowUnverifiedPeer
            self.timeout = timeout
        }

        /// Diagnostics role: no signing identity required, never `local_ui`, never OS-capable
        /// services, and no OS capability in the hello.
        public static func phase0Diagnostics(socketPath: String = HostIPCPaths.socketPath) -> Configuration {
            Configuration(
                socketPath: socketPath,
                role: HostIPCRole.diagnostics,
                capabilities: ["host", "native_host_smoke"],
                expectedDaemon: HostIPCPaths.expectedDaemonSignature(),
                allowUnverifiedPeer: HostIPCPaths.allowsUnverifiedPeer
            )
        }
    }

    public static var defaultAppVersion: String {
        (Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String) ?? "0.1.0-phase0"
    }

    public static var defaultBundleIdentifier: String {
        Bundle.main.bundleIdentifier ?? "com.codebridge.app"
    }

    public let configuration: Configuration
    public private(set) var negotiatedProtocol: ProtocolVersion?
    public private(set) var daemonVersion: String?
    public private(set) var helloResult: HostHelloResult?
    public private(set) var peerVerification: SignatureVerification?
    public private(set) var isConnected = false

    private var connection: UnixSocketConnection?
    private var decoder = FrameDecoder()
    private var nextRequestID = 1

    public init(configuration: Configuration = Configuration()) {
        self.configuration = configuration
    }

    deinit {
        close()
    }

    public var peerCredentials: PeerCredentials? {
        connection?.peerCredentials
    }

    /// Connects, checks peer UID/signature, and runs `host.hello`.
    @discardableResult
    public func connect() throws -> HostHelloResult {
        close()
        let connection: UnixSocketConnection
        do {
            connection = try UnixSocketConnection.connect(path: configuration.socketPath, timeout: configuration.timeout)
        } catch let error as UnixSocketError {
            throw HostIPCError.transport(error.description)
        }
        self.connection = connection
        self.decoder = FrameDecoder()

        guard let credentials = connection.peerCredentials else {
            close()
            throw HostIPCError.transport("peer credentials unavailable")
        }
        let ownUID = getuid()
        guard credentials.uid == ownUID else {
            close()
            throw HostIPCError.peerUIDMismatch(expected: ownUID, actual: credentials.uid)
        }

        // Peer trust policy: role=app is never allowed to skip verification, with or without the
        // environment opt-in (which exists only for Phase 0 diagnostics/harness).
        let decision = PeerTrustPolicy.decide(
            role: configuration.role,
            expectedDaemon: configuration.expectedDaemon,
            allowUnverifiedPeer: configuration.allowUnverifiedPeer
        )
        switch decision {
        case .refuse(let reason):
            peerVerification = .unverifiable(
                reason: reason,
                actual: PeerIdentity.inspect(fileDescriptor: connection.fileDescriptor)
            )
            close()
            throw HostIPCError.unverifiedPeer(reason)
        case .allowUnverifiedDiagnosticsPeer(let reason):
            peerVerification = .unverifiable(
                reason: reason,
                actual: PeerIdentity.inspect(fileDescriptor: connection.fileDescriptor)
            )
        case .requireVerifiedPeer(let expected):
            let verification = PeerIdentity.verify(fileDescriptor: connection.fileDescriptor, expected: expected)
            peerVerification = verification
            guard verification.isVerified else {
                close()
                throw HostIPCError.unverifiedPeer(verification.summary)
            }
            // 2026-10-03 amendment: the daemon must be the EXTERNALLY INSTALLED copy
            // (F2 topology). A verified daemon whose executable still lives inside this
            // app bundle is the legacy bundled topology and must be refused.
            if expected.signingIdentifier == "com.codebridge.daemon",
               let path = verification.verifiedIdentity?.executablePath,
               path.hasPrefix(Bundle.main.bundlePath + "/") {
                close()
                throw HostIPCError.unverifiedPeer(
                    "daemon peer is the bundled legacy copy; production topology requires the external install"
                )
            }
        }

        let params = HostHelloParams(
            protocol: .current,
            appVersion: configuration.appVersion,
            role: configuration.role,
            capabilities: configuration.capabilities,
            bundleIdentifier: configuration.bundleIdentifier,
            pid: getpid()
        )
        let resultValue: JSONValue
        do {
            resultValue = try call(method: HostIPCMethod.hello, params: try JSONValue.from(params), timeout: configuration.timeout)
        } catch {
            close()
            throw error
        }
        let result: HostHelloResult
        do {
            result = try resultValue.decoded(as: HostHelloResult.self)
        } catch {
            close()
            throw HostIPCError.malformedResponse("host.hello result: \(error)")
        }
        helloResult = result
        guard result.accepted else {
            close()
            throw HostIPCError.handshakeRejected(result.reason ?? "daemon did not accept the handshake")
        }
        switch ProtocolRules.negotiate(client: ProtocolVersion.current, daemon: result.protocol) {
        case .accepted(let negotiated):
            negotiatedProtocol = negotiated
        case .rejectedMajorMismatch, .rejectedMinorUnsupported:
            close()
            throw HostIPCError.protocolRejected(
                result.reason ?? "client \(ProtocolVersion.current) vs daemon \(result.protocol)"
            )
        }
        daemonVersion = result.daemon_version
        isConnected = true
        return result
    }

    public func health() throws -> JSONValue {
        try call(method: HostIPCMethod.health, params: nil, timeout: configuration.timeout)
    }

    public func prepareRestart() throws -> JSONValue {
        try call(method: HostIPCMethod.prepareRestart, params: nil, timeout: configuration.timeout)
    }

    /// Phase 0 TCC-attribution probe: the daemon spawns the probe binary as **its own child**.
    public func phase0Probe(
        _ probe: String,
        args: [String] = ["--json"],
        timeout: TimeInterval = 60
    ) throws -> Phase0ProbeResult {
        let params = Phase0ProbeParams(probe: probe, args: args, timeoutMilliseconds: Int(timeout * 1000))
        let value = try call(
            method: HostIPCMethod.phase0Probe,
            params: try JSONValue.from(params),
            timeout: timeout + 5
        )
        do {
            return try value.decoded(as: Phase0ProbeResult.self)
        } catch {
            throw HostIPCError.malformedResponse("host.phase0_probe result: \(error)")
        }
    }

    /// Daemon-side native host smoke (fixed argv set owned by the daemon).
    public func nativeHostSmoke(only: [String]? = nil) throws -> JSONValue {
        let params = NativeHostSmokeParams(only: only)
        return try call(
            method: HostIPCMethod.nativeHostSmoke,
            params: try JSONValue.from(params),
            timeout: configuration.timeout + 15
        )
    }

    public func close() {
        isConnected = false
        connection?.close()
        connection = nil
    }

    // MARK: - Private

    private func call(method: String, params: JSONValue?, timeout: TimeInterval) throws -> JSONValue {
        guard let connection = connection, connection.isOpen else {
            throw HostIPCError.notConnected
        }
        let identifier = nextRequestID
        nextRequestID += 1
        let request = JSONRPCRequest(id: identifier, method: method, params: params)
        let payload = try JSONEncoder().encode(request)
        do {
            try connection.writeAll(try FrameCodec.encode(payload, kind: .json))
        } catch let error as UnixSocketError {
            throw HostIPCError.transport(error.description)
        }

        let deadline = Date().addingTimeInterval(timeout)
        while true {
            let remaining = deadline.timeIntervalSinceNow
            if remaining <= 0 {
                throw HostIPCError.timedOut(method: method)
            }
            let chunk: Data
            do {
                chunk = try connection.readSome(timeout: min(remaining, 2))
            } catch let error as UnixSocketError {
                switch error {
                case .timedOut:
                    continue
                case .closed:
                    throw HostIPCError.transport("closed by daemon")
                default:
                    throw HostIPCError.transport(error.description)
                }
            }
            guard !chunk.isEmpty else { continue }
            decoder.append(chunk)
            while let frame = try decoder.nextFrame() {
                guard frame.kind == .json else {
                    throw HostIPCError.unexpectedBinaryFrame
                }
                guard let response = try? JSONDecoder().decode(JSONRPCResponse.self, from: frame.payload) else {
                    continue
                }
                guard response.id == identifier else {
                    continue
                }
                if let error = response.error {
                    throw HostIPCError.rpc(
                        code: error.code,
                        name: HostIPCErrorCode.name(for: error.code),
                        message: error.message
                    )
                }
                guard let result = response.result else {
                    throw HostIPCError.malformedResponse("response without result for \(method)")
                }
                return result
            }
        }
    }
}
