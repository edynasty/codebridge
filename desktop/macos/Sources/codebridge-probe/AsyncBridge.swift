import Foundation

/// Bridges an `async` ScreenCaptureKit call into this synchronous CLI without blocking a
/// cooperative-pool thread indefinitely.
final class AsyncResultBox<T> {
    var value: Result<T, Error>?
}

func runAsync<T>(
    timeout: TimeInterval = 30,
    _ operation: @escaping () async throws -> T
) -> Result<T, Error> {
    let box = AsyncResultBox<T>()
    let semaphore = DispatchSemaphore(value: 0)
    Task.detached {
        do {
            box.value = .success(try await operation())
        } catch {
            box.value = .failure(error)
        }
        semaphore.signal()
    }
    if semaphore.wait(timeout: .now() + timeout) == .timedOut {
        return .failure(ProbeCLIError.internalFailure("async operation timed out after \(Int(timeout))s"))
    }
    guard let value = box.value else {
        return .failure(ProbeCLIError.internalFailure("async operation produced no result"))
    }
    return value
}
