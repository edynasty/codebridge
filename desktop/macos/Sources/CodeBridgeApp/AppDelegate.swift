import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var menuBarController: MenuBarController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        let controller = MenuBarController()
        controller.install()
        menuBarController = controller

        if CommandLine.arguments.contains("--phase0b-ax-diagnostic") {
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) {
                Self.runAXBaselineDiagnostic()
            }
        }
        if CommandLine.arguments.contains("--phase0b-request-permission") {
            DispatchQueue.main.async {
                do {
                    try Phase0BAcceptance.requestComputerPermission(arguments: CommandLine.arguments)
                } catch {
                    FileHandle.standardError.write(Data(("CodeBridge permission request: \(error)\n").utf8))
                    NSApplication.shared.terminate(nil)
                }
            }
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        menuBarController?.shutdown()
    }

    /// Real AX acceptance must run in a live App with an active NSApplication run loop: AX
    /// messaging replies are delivered through the main run loop, and the previous immediate-exit
    /// probe process could never receive them. This path keeps the App alive long enough to
    /// complete all reads, then terminates.
    private static func runAXBaselineDiagnostic() {
        let arguments = CommandLine.arguments
        let report = AXBaselineDiagnostic.run(observationSeconds: 2)
        if let index = arguments.firstIndex(of: "--output"), index + 1 < arguments.count {
            let encoder = JSONEncoder()
            encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
            if let data = try? encoder.encode(report) {
                try? data.write(to: URL(fileURLWithPath: arguments[index + 1]), options: .atomic)
                FileHandle.standardOutput.write(data)
                FileHandle.standardOutput.write(Data("\n".utf8))
            } else {
                FileHandle.standardError.write(Data("CodeBridge AX diagnostic: report encoding failed\n".utf8))
            }
        } else {
            FileHandle.standardError.write(Data("CodeBridge AX diagnostic: --output missing\n".utf8))
        }
        NSApplication.shared.terminate(nil)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }
}
