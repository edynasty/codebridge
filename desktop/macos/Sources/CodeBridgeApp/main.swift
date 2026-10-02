import AppKit
import Foundation
import CodeBridgeNativeProbe

/// CodeBridge.app — menu-bar host adapter (Phase 0).
///
/// Normal start: accessory (menu-bar only) app; no production Computer Use engine. The opt-in
/// `--phase0b-probe` path calls the shared probe in this process (matrix A), never as an App child.
///
/// Scriptable Phase 0 path — `CodeBridge --phase0-probe <probe>` runs the bundled
/// `codebridge-probe` **as a child of this app process** and exits. Used for the TCC attribution
/// spike, launched through LaunchServices so the app (not a shell) is the responsible process:
///
///   open -a CodeBridge.app --args --phase0-probe permissions
///
/// The raw probe JSON is written under `~/Library/Logs/CodeBridge/`.
let arguments = CommandLine.arguments
if arguments.contains("--phase0b-probe") || arguments.contains("--phase0b-service") {
    do {
        exit(try Phase0BAcceptance.run(arguments: arguments))
    } catch let error as ProbeCLIError {
        FileHandle.standardError.write(Data(("CodeBridge: \(error.description)\n").utf8))
        exit(error.exitCode)
    } catch {
        FileHandle.standardError.write(Data(("CodeBridge: \(error)\n").utf8))
        exit(3)
    }
}
if let flagIndex = arguments.firstIndex(of: "--phase0-probe"), flagIndex + 1 < arguments.count {
    let probe = arguments[flagIndex + 1]
    let outcome = ProbeChildRunner.run(probe: probe)
    FileHandle.standardOutput.write(Data((outcome.body + "\n").utf8))
    exit(outcome.ok ? 0 : 1)
}

let application = NSApplication.shared
let delegate = AppDelegate()
application.delegate = delegate
_ = application.setActivationPolicy(.accessory)
application.run()
