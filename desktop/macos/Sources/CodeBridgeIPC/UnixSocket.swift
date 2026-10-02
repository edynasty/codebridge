import Darwin
import Foundation

public enum UnixSocketError: Error, Equatable, CustomStringConvertible {
    case invalidPath(String)
    case socketCreationFailed(errno: Int32)
    case connectFailed(path: String, errno: Int32)
    case writeFailed(errno: Int32)
    case readFailed(errno: Int32)
    case timedOut(String)
    case closed

    public var description: String {
        switch self {
        case .invalidPath(let path):
            return "invalid socket path: \(path)"
        case .socketCreationFailed(let code):
            return "socket(2) failed (errno \(code))"
        case .connectFailed(let path, let code):
            return "connect(2) to \(path) failed (errno \(code))"
        case .writeFailed(let code):
            return "write(2) failed (errno \(code))"
        case .readFailed(let code):
            return "read(2) failed (errno \(code))"
        case .timedOut(let operation):
            return "\(operation) timed out"
        case .closed:
            return "socket closed by peer"
        }
    }
}

/// Blocking Unix-domain stream socket with poll-based timeouts.
///
/// The Host IPC transport is a Unix-domain socket in a 0700 per-user directory; the daemon listens
/// and the app connects (`docs/v2/provider-contracts.md` §7).
public final class UnixSocketConnection {
    public let path: String
    public let fileDescriptor: Int32
    private var isClosed = false

    public init(fileDescriptor: Int32, path: String) {
        self.fileDescriptor = fileDescriptor
        self.path = path
    }

    deinit {
        close()
    }

    public static func connect(path: String, timeout: TimeInterval = 5) throws -> UnixSocketConnection {
        let pathBytes = Array(path.utf8)
        var address = sockaddr_un()
        let sunPathSize = MemoryLayout.size(ofValue: address.sun_path)
        guard pathBytes.count < sunPathSize else {
            throw UnixSocketError.invalidPath(path)
        }
        address.sun_family = sa_family_t(AF_UNIX)
        address.sun_len = UInt8(MemoryLayout<sockaddr_un>.size)
        withUnsafeMutableBytes(of: &address.sun_path) { destination in
            destination.copyBytes(from: pathBytes)
        }

        let descriptor = socket(AF_UNIX, SOCK_STREAM, 0)
        guard descriptor >= 0 else {
            throw UnixSocketError.socketCreationFailed(errno: errno)
        }

        setNonBlocking(descriptor)
        var result: Int32 = -1
        withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { socketAddress in
                result = Darwin.connect(descriptor, socketAddress, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        if result != 0 {
            let code = errno
            if code == EINPROGRESS {
                var descriptorState = pollfd(fd: descriptor, events: Int16(POLLOUT), revents: 0)
                let milliseconds = Int32(max(0, timeout) * 1000)
                let pollResult = Darwin.poll(&descriptorState, 1, milliseconds)
                if pollResult <= 0 {
                    Darwin.close(descriptor)
                    throw UnixSocketError.connectFailed(path: path, errno: ETIMEDOUT)
                }
                var socketError: Int32 = 0
                var length = socklen_t(MemoryLayout<Int32>.size)
                getsockopt(descriptor, SOL_SOCKET, SO_ERROR, &socketError, &length)
                if socketError != 0 {
                    Darwin.close(descriptor)
                    throw UnixSocketError.connectFailed(path: path, errno: socketError)
                }
            } else {
                Darwin.close(descriptor)
                throw UnixSocketError.connectFailed(path: path, errno: code)
            }
        }

        return UnixSocketConnection(fileDescriptor: descriptor, path: path)
    }

    private static func setNonBlocking(_ descriptor: Int32) {
        let flags = fcntl(descriptor, F_GETFL, 0)
        if flags >= 0 {
            _ = fcntl(descriptor, F_SETFL, flags | O_NONBLOCK)
        }
    }

    public func close() {
        guard !isClosed else { return }
        isClosed = true
        Darwin.close(fileDescriptor)
    }

    public var isOpen: Bool { !isClosed }

    /// Peer credentials of the connected socket (peer UID check, audit token for signature checks).
    public var peerCredentials: PeerCredentials? {
        PeerIdentity.credentials(fileDescriptor: fileDescriptor)
    }

    private func waitFor(_ events: Int16, timeout: TimeInterval) -> Bool {
        var descriptor = pollfd(fd: fileDescriptor, events: events, revents: 0)
        let clamped = max(0, min(timeout, 3600))
        let milliseconds = Int32(clamped * 1000)
        while true {
            let result = Darwin.poll(&descriptor, 1, milliseconds)
            if result < 0 {
                if errno == EINTR { continue }
                return false
            }
            return result > 0
        }
    }

    public func writeAll(_ data: Data) throws {
        guard !isClosed else { throw UnixSocketError.closed }
        let bytes = [UInt8](data)
        var offset = 0
        while offset < bytes.count {
            guard waitFor(Int16(POLLOUT), timeout: 5) else {
                throw UnixSocketError.timedOut("write")
            }
            let written: Int = bytes.withUnsafeBytes { raw -> Int in
                guard let base = raw.baseAddress else { return 0 }
                return Darwin.write(fileDescriptor, base.advanced(by: offset), bytes.count - offset)
            }
            if written < 0 {
                if errno == EINTR || errno == EAGAIN { continue }
                throw UnixSocketError.writeFailed(errno: errno)
            }
            if written == 0 {
                throw UnixSocketError.writeFailed(errno: 0)
            }
            offset += written
        }
    }

    /// Reads whatever is available; throws `.closed` on EOF.
    public func readSome(timeout: TimeInterval) throws -> Data {
        guard !isClosed else { throw UnixSocketError.closed }
        guard waitFor(Int16(POLLIN), timeout: timeout) else {
            throw UnixSocketError.timedOut("read")
        }
        var buffer = [UInt8](repeating: 0, count: 64 * 1024)
        let count = Darwin.read(fileDescriptor, &buffer, buffer.count)
        if count < 0 {
            if errno == EAGAIN || errno == EINTR { return Data() }
            throw UnixSocketError.readFailed(errno: errno)
        }
        if count == 0 {
            throw UnixSocketError.closed
        }
        return Data(buffer[0..<count])
    }
}
