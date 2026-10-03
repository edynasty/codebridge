import AppKit
import ApplicationServices
import CodeBridgeIPC
import Foundation

/// Phase 0B AX acceptance — diagnostic paths for a real, LaunchServices-launched App main process.
///
/// Reports three real AX read paths plus full caller identity. Read-only: no AX writes, no UI
/// automation, no keystroke or content capture.
enum AXBaselineDiagnostic {
    struct Report: Codable {
        let schema = "codebridge.phase0b.ax_baseline_diagnostic.v1"
        let pid: Int32
        let ppid: Int32
        let executablePath: String?
        let bundleIdentifier: String?
        let teamIdentifier: String?
        let cdhash: String?
        let isAdHoc: Bool?
        let startedAt: String
        var endedAt: String?
        let observationSeconds: Int
        var trusted: Bool?
        var pathA: PathResult?
        var pathB: PathResult?
        var pathC: PathCResult?
        var notes: [String] = []
    }

    struct PathResult: Codable {
        var api: String
        var result: String
        var errorCode: Int32?
        var detail: String?
        var elementValid: Bool?
    }

    struct PathCResult: Codable {
        var frontmostPid: Int32?
        var frontmostBundleIdentifier: String?
        var frontmostLocalizedName: String?
        var createApplicationResult: String
        var roleRead: PathResult?
        var titleRead: PathResult?
    }

    /// Runs on the App's main thread while the NSApplication run loop is alive.
    static func run(observationSeconds: Int) -> Report {
        let identity = ProcessIdentityReader.current()
        let signing = PeerIdentity.signingIdentity(pid: getpid())
        var report = Report(
            pid: getpid(),
            ppid: identity.parentPID,
            executablePath: identity.executablePath,
            bundleIdentifier: Bundle.main.bundleIdentifier,
            teamIdentifier: signing?.teamIdentifier,
            cdhash: signing?.cdhashHex,
            isAdHoc: signing?.isAdHoc,
            startedAt: ISO8601DateFormatter().string(from: Date()),
            observationSeconds: observationSeconds
        )

        report.trusted = AXIsProcessTrusted()

        // Path A: trust check only (never sufficient for PASS).
        report.pathA = PathResult(
            api: "AXIsProcessTrusted", result: report.trusted == true ? "ok" : "denied",
            errorCode: nil, detail: nil, elementValid: nil
        )

        // Path B: SystemWide focused application — the historically failing API path.
        let systemWide = AXUIElementCreateSystemWide()
        var focused: CFTypeRef?
        let status = AXUIElementCopyAttributeValue(systemWide, "AXFocusedApplication" as CFString, &focused)
        if status == .success, let element = focused {
            report.pathB = PathResult(
                api: "AXUIElementCreateSystemWide + AXFocusedApplication",
                result: "ok", errorCode: status.rawValue,
                detail: "CFTypeID=\(CFGetTypeID(element))",
                elementValid: CFGetTypeID(element) == AXUIElementGetTypeID()
            )
        } else {
            report.pathB = PathResult(
                api: "AXUIElementCreateSystemWide + AXFocusedApplication",
                result: "error", errorCode: status.rawValue,
                detail: "kAXError raw \(status.rawValue)", elementValid: false
            )
        }

        // Path C: NSWorkspace frontmost → application-scoped AXUIElement read.
        report.pathC = readFrontmostViaNSWorkspace()

        report.endedAt = ISO8601DateFormatter().string(from: Date())
        return report
    }

    private static func readFrontmostViaNSWorkspace() -> PathCResult {
        var result = PathCResult(
            frontmostPid: nil, frontmostBundleIdentifier: nil, frontmostLocalizedName: nil,
            createApplicationResult: "not_attempted", roleRead: nil, titleRead: nil
        )
        guard let frontmost = NSWorkspace.shared.frontmostApplication else {
            result.createApplicationResult = "no_frontmost_application"
            return result
        }
        result.frontmostPid = frontmost.processIdentifier
        result.frontmostBundleIdentifier = frontmost.bundleIdentifier
        result.frontmostLocalizedName = frontmost.localizedName

        let application = AXUIElementCreateApplication(frontmost.processIdentifier)
        result.createApplicationResult = "created"

        var role: CFTypeRef?
        let roleStatus = AXUIElementCopyAttributeValue(application, "AXRole" as CFString, &role)
        if roleStatus == .success, let value = role {
            result.roleRead = PathResult(
                api: "AXUIElementCreateApplication(frontmost) + AXRole",
                result: "ok", errorCode: roleStatus.rawValue,
                detail: "CFTypeID=\(CFGetTypeID(value))",
                elementValid: true
            )
        } else {
            result.roleRead = PathResult(
                api: "AXUIElementCreateApplication(frontmost) + AXRole",
                result: "error", errorCode: roleStatus.rawValue,
                detail: "kAXError raw \(roleStatus.rawValue)", elementValid: false
            )
        }

        var title: CFTypeRef?
        let titleStatus = AXUIElementCopyAttributeValue(application, "AXTitle" as CFString, &title)
        if titleStatus == .success, let value = title as? String {
            result.titleRead = PathResult(
                api: "AXUIElementCreateApplication(frontmost) + AXTitle",
                result: "ok", errorCode: titleStatus.rawValue,
                detail: "title length read (content not required for acceptance)",
                elementValid: true
            )
        } else {
            result.titleRead = PathResult(
                api: "AXUIElementCreateApplication(frontmost) + AXTitle",
                result: "error", errorCode: titleStatus.rawValue,
                detail: "kAXError raw \(titleStatus.rawValue)", elementValid: false
            )
        }
        return result
    }
}
