import Foundation

/// Host IPC frame kinds (frozen with `codebridged`, see `schema/hostipc/v1/README.md`).
///
/// ```text
/// frame := u32be header || payload
/// header bit31 = 0 -> JSON control frame, len = payload bytes, payload = UTF-8 JSON-RPC 2.0 object
/// header bit31 = 1 -> binary attachment, len = payload bytes,
///                    payload = u32be metaLen || meta JSON || raw bytes
/// ```
public enum FrameKind: String, Equatable, Sendable {
    case json
    case binaryAttachment
}

public struct Frame: Equatable, Sendable {
    public let kind: FrameKind
    public let payload: Data

    public init(kind: FrameKind, payload: Data) {
        self.kind = kind
        self.payload = payload
    }
}

public enum FramingError: Error, Equatable, CustomStringConvertible {
    case invalidHeaderLength(Int)
    case frameTooLarge(kind: FrameKind, bytes: Int, limit: Int)
    case truncatedFrame
    case malformedAttachment

    public var description: String {
        switch self {
        case .invalidHeaderLength(let count):
            return "invalid frame header length \(count)"
        case .frameTooLarge(let kind, let bytes, let limit):
            return "\(kind.rawValue) frame of \(bytes) bytes exceeds the \(limit) byte limit"
        case .truncatedFrame:
            return "truncated frame"
        case .malformedAttachment:
            return "malformed binary attachment frame"
        }
    }
}

/// Attachment metadata carried in a binary attachment frame.
public struct AttachmentMeta: Codable, Equatable, Sendable {
    public var attachmentID: String
    public var mime: String
    public var bytes: Int
    public var sha256: String?
    public var role: String?

    enum CodingKeys: String, CodingKey {
        case attachmentID = "attachment_id"
        case mime
        case bytes
        case sha256
        case role
    }

    public init(attachmentID: String, mime: String, bytes: Int, sha256: String? = nil, role: String? = nil) {
        self.attachmentID = attachmentID
        self.mime = mime
        self.bytes = bytes
        self.sha256 = sha256
        self.role = role
    }
}

public enum FrameCodec {
    public static let headerLength = 4
    public static let binaryFlag: UInt32 = 0x8000_0000
    public static let lengthMask: UInt32 = 0x7fff_ffff
    /// Control (JSON) frames are bounded at 1 MiB by the daemon-owned Phase 0 schema.
    public static let maxJSONFrameBytes = 1 * 1024 * 1024
    /// Binary attachment frames are bounded at 16 MiB.
    public static let maxAttachmentFrameBytes = 16 * 1024 * 1024

    public static func limit(for kind: FrameKind) -> Int {
        kind == .json ? maxJSONFrameBytes : maxAttachmentFrameBytes
    }

    public static func encode(_ payload: Data, kind: FrameKind = .json) throws -> Data {
        let limit = limit(for: kind)
        guard payload.count <= limit else {
            throw FramingError.frameTooLarge(kind: kind, bytes: payload.count, limit: limit)
        }
        var header = UInt32(payload.count)
        if kind == .binaryAttachment {
            header |= binaryFlag
        }
        var out = Data(capacity: headerLength + payload.count)
        out.append(UInt8((header >> 24) & 0xff))
        out.append(UInt8((header >> 16) & 0xff))
        out.append(UInt8((header >> 8) & 0xff))
        out.append(UInt8(header & 0xff))
        out.append(payload)
        return out
    }

    public static func encodeJSON(_ value: JSONValue) throws -> Data {
        try encode(try value.encoded(), kind: .json)
    }

    public static func encodeAttachment(meta: AttachmentMeta, payload: Data) throws -> Data {
        let metaData = try JSONEncoder().encode(meta)
        var body = Data(capacity: 4 + metaData.count + payload.count)
        let metaLength = UInt32(metaData.count)
        body.append(UInt8((metaLength >> 24) & 0xff))
        body.append(UInt8((metaLength >> 16) & 0xff))
        body.append(UInt8((metaLength >> 8) & 0xff))
        body.append(UInt8(metaLength & 0xff))
        body.append(metaData)
        body.append(payload)
        return try encode(body, kind: .binaryAttachment)
    }

    public static func decodeHeader(_ header: Data) throws -> (kind: FrameKind, length: Int) {
        guard header.count == headerLength else {
            throw FramingError.invalidHeaderLength(header.count)
        }
        let bytes = [UInt8](header)
        let raw = (UInt32(bytes[0]) << 24) | (UInt32(bytes[1]) << 16) | (UInt32(bytes[2]) << 8) | UInt32(bytes[3])
        let kind: FrameKind = (raw & binaryFlag) != 0 ? .binaryAttachment : .json
        let length = Int(raw & lengthMask)
        let limit = limit(for: kind)
        guard length <= limit else {
            throw FramingError.frameTooLarge(kind: kind, bytes: length, limit: limit)
        }
        return (kind, length)
    }

    public static func decodeAttachment(_ payload: Data) throws -> (meta: AttachmentMeta, body: Data) {
        guard payload.count >= 4 else { throw FramingError.malformedAttachment }
        let bytes = [UInt8](payload)
        let metaLength = Int((UInt32(bytes[0]) << 24) | (UInt32(bytes[1]) << 16) | (UInt32(bytes[2]) << 8) | UInt32(bytes[3]))
        guard metaLength >= 0, payload.count >= 4 + metaLength else { throw FramingError.malformedAttachment }
        let metaStart = payload.index(payload.startIndex, offsetBy: 4)
        let metaEnd = payload.index(metaStart, offsetBy: metaLength)
        let metaData = Data(payload[metaStart..<metaEnd])
        let meta = try JSONDecoder().decode(AttachmentMeta.self, from: metaData)
        return (meta, Data(payload[metaEnd...]))
    }
}

/// Incremental frame reader for a byte stream.
public struct FrameDecoder {
    private var buffer = Data()

    public init() {}

    public mutating func append(_ data: Data) {
        buffer.append(data)
    }

    public var bufferedByteCount: Int { buffer.count }

    /// Returns the next complete frame, or `nil` when more bytes are required.
    public mutating func nextFrame() throws -> Frame? {
        guard buffer.count >= FrameCodec.headerLength else { return nil }
        let headerEnd = buffer.index(buffer.startIndex, offsetBy: FrameCodec.headerLength)
        let (kind, length) = try FrameCodec.decodeHeader(Data(buffer[buffer.startIndex..<headerEnd]))
        guard buffer.count >= FrameCodec.headerLength + length else { return nil }
        let payloadStart = headerEnd
        let payloadEnd = buffer.index(payloadStart, offsetBy: length)
        let payload = Data(buffer[payloadStart..<payloadEnd])
        buffer.removeSubrange(buffer.startIndex..<payloadEnd)
        return Frame(kind: kind, payload: payload)
    }
}
