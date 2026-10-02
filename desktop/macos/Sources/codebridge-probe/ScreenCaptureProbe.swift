import CodeBridgeComputerSpike
import CoreGraphics
import CryptoKit
import Foundation
import ScreenCaptureKit

/// ScreenCaptureKit probe: preflight, real `SCShareableContent` call, and one real still capture.
///
/// Pixels never touch the disk and never appear in the report — only dimensions and a SHA-256 of
/// the in-memory buffer.
enum ScreenCaptureProbe {
    static func run() -> ScreenCaptureProbeReport {
        let preflight = CGPreflightScreenCaptureAccess()
        var report = ScreenCaptureProbeReport(
            preflight_granted: preflight,
            shareable_content_call: "skipped",
            shareable_content_error: nil,
            display_count: nil,
            capture_call: "skipped",
            capture_error: nil,
            captured_width: nil,
            captured_height: nil,
            captured_sha256: nil,
            pixels_persisted: false
        )

        // The real call is always attempted: a denied call is evidence, and it never prompts.
        let contentResult = runAsync {
            try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: true)
        }
        let content: SCShareableContent
        switch contentResult {
        case .success(let value):
            report.shareable_content_call = "ok"
            report.display_count = value.displays.count
            content = value
        case .failure(let error):
            report.shareable_content_call = preflight ? "error" : "denied"
            report.shareable_content_error = describe(error)
            return report
        }

        guard let display = content.displays.first else {
            report.capture_call = "no_display"
            return report
        }

        let filter = SCContentFilter(display: display, excludingWindows: [])
        let configuration = SCStreamConfiguration()
        configuration.width = display.width
        configuration.height = display.height
        configuration.showsCursor = false

        guard #available(macOS 14.0, *) else {
            report.capture_call = "unsupported_os_version"
            report.capture_error = "SCScreenshotManager requires macOS 14 or newer"
            return report
        }

        let captureResult = runAsync {
            try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: configuration)
        }
        switch captureResult {
        case .success(let image):
            report.capture_call = "ok"
            if let summary = digest(of: image) {
                report.captured_width = summary.width
                report.captured_height = summary.height
                report.captured_sha256 = summary.digest
            }
        case .failure(let error):
            report.capture_call = "error"
            report.capture_error = describe(error)
        }
        return report
    }

    private static func describe(_ error: Error) -> String {
        let nsError = error as NSError
        return "\(nsError.domain) \(nsError.code): \(nsError.localizedDescription)"
    }

    private static func digest(of image: CGImage) -> (width: Int, height: Int, digest: String)? {
        let width = image.width
        let height = image.height
        guard width > 0, height > 0 else { return nil }
        let bytesPerRow = width * 4
        var buffer = [UInt8](repeating: 0, count: bytesPerRow * height)
        let colorSpace = CGColorSpaceCreateDeviceRGB()
        let bitmapInfo = CGImageAlphaInfo.premultipliedLast.rawValue | CGBitmapInfo.byteOrder32Big.rawValue
        let created: Bool = buffer.withUnsafeMutableBytes { raw -> Bool in
            guard let context = CGContext(
                data: raw.baseAddress,
                width: width,
                height: height,
                bitsPerComponent: 8,
                bytesPerRow: bytesPerRow,
                space: colorSpace,
                bitmapInfo: bitmapInfo
            ) else { return false }
            context.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
            return true
        }
        guard created else { return nil }
        let digest = SHA256.hash(data: Data(buffer))
        return (width, height, digest.map { String(format: "%02x", $0) }.joined())
    }
}
