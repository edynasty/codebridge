import Foundation

/// JSON-RPC 2.0 error object. `JSONRPCErrorCode` carries the CodeBridge Host IPC table.
public struct JSONRPCError: Codable, Equatable, Sendable, Error, CustomStringConvertible {
    public var code: Int
    public var message: String
    public var data: JSONValue?

    public init(code: Int, message: String, data: JSONValue? = nil) {
        self.code = code
        self.message = message
        self.data = data
    }

    public var description: String { "JSON-RPC error \(code): \(message)" }
}

/// Error codes on the CodeBridge Host IPC wire (frozen with `codebridged` 2026-10-02).
public enum HostIPCErrorCode {
    public static let parse = -32700
    public static let invalidRequest = -32600
    public static let methodNotFound = -32601
    public static let invalidParams = -32602
    public static let internalError = -32603
    public static let protocolMajorMismatch = -32000
    public static let protocolMinorUnsupported = -32001
    public static let notInitialized = -32002
    public static let unauthorizedPeer = -32010
    public static let roleForbidden = -32011
    public static let frameLimit = -32020

    public static func name(for code: Int) -> String {
        switch code {
        case parse: return "parse"
        case invalidRequest: return "invalid_request"
        case methodNotFound: return "unsupported"
        case invalidParams: return "invalid_params"
        case internalError: return "internal"
        case protocolMajorMismatch: return "protocol_major_mismatch"
        case protocolMinorUnsupported: return "protocol_minor_unsupported"
        case notInitialized: return "not_initialized"
        case unauthorizedPeer: return "unauthorized_peer"
        case roleForbidden: return "role_forbidden"
        case frameLimit: return "frame_limit"
        default: return "unknown"
        }
    }
}

public struct JSONRPCRequest: Codable, Equatable, Sendable {
    public var jsonrpc: String
    public var id: Int
    public var method: String
    public var params: JSONValue?

    public init(id: Int, method: String, params: JSONValue? = nil) {
        self.jsonrpc = "2.0"
        self.id = id
        self.method = method
        self.params = params
    }
}

public struct JSONRPCResponse: Codable, Equatable, Sendable {
    public var jsonrpc: String
    public var id: Int?
    public var result: JSONValue?
    public var error: JSONRPCError?

    public init(id: Int?, result: JSONValue? = nil, error: JSONRPCError? = nil) {
        self.jsonrpc = "2.0"
        self.id = id
        self.result = result
        self.error = error
    }

    /// Lenient decoding: unknown fields are ignored (additive minor changes) and a missing
    /// `jsonrpc` field does not fail the read.
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.jsonrpc = (try? container.decodeIfPresent(String.self, forKey: .jsonrpc)) ?? "2.0"
        self.id = try? container.decodeIfPresent(Int.self, forKey: .id)
        self.result = try? container.decodeIfPresent(JSONValue.self, forKey: .result)
        self.error = try? container.decodeIfPresent(JSONRPCError.self, forKey: .error)
    }

    private enum CodingKeys: String, CodingKey {
        case jsonrpc, id, result, error
    }
}

/// Daemon → app notification (no `id`). Phase 0: declared, not yet produced.
public struct JSONRPCNotification: Codable, Equatable, Sendable {
    public var jsonrpc: String
    public var method: String
    public var params: JSONValue?

    public init(method: String, params: JSONValue? = nil) {
        self.jsonrpc = "2.0"
        self.method = method
        self.params = params
    }
}
