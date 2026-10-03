import ApplicationServices
import CodeBridgeComputerSpike
import Foundation

/// Accessibility probe: trust check plus a real AX API call (`AXFocusedApplication` on the
/// system-wide element). Never uses the prompting variant of the trust check.
enum AccessibilityProbe {
    static func run() -> AccessibilityProbeReport {
        let trusted = AXIsProcessTrusted()
        let systemWide = AXUIElementCreateSystemWide()
        var value: CFTypeRef?
        // Literal attribute name avoids depending on how the CFSTR macro is imported.
        let status = AXUIElementCopyAttributeValue(systemWide, "AXFocusedApplication" as CFString, &value)
        let call: String
        var detail: String?
        switch status {
        case .success:
            // Success must include an actual focused application, not an empty API result.
            if let value, CFGetTypeID(value) == AXUIElementGetTypeID() {
                call = "ok"
            } else {
                call = "error"
                detail = "AXFocusedApplication returned no valid AXUIElement"
            }
        case .apiDisabled:
            call = "denied"
            detail = "kAXErrorAPIDisabled: accessibility is not granted to this process"
        case .notImplemented:
            call = "unsupported"
            detail = "kAXErrorNotImplemented"
        case .cannotComplete:
            call = "error"
            detail = "kAXErrorCannotComplete"
        case .invalidUIElement:
            call = "error"
            detail = "kAXErrorInvalidUIElement"
        default:
            call = "error"
            detail = "AXError raw value \(status.rawValue)"
        }
        return AccessibilityProbeReport(
            trusted: trusted,
            focused_application_call: call,
            focused_application_error: detail,
            api_error_code: status.rawValue
        )
    }
}
