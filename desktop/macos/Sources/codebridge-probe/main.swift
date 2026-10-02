import Foundation
import CodeBridgeNativeProbe

/// `codebridge-probe` — standalone Phase 0 permission / lock-state / input-monitor harness.
///
/// Invocation (frozen with the daemon): `<probe> <probeName> --json`
///
/// Safety invariants (enforced here, asserted by the evidence):
/// - never requests a TCC permission (no `CGRequest*Access`, no AX prompt option);
/// - never posts an input event and never creates an event tap that can modify events;
/// - never persists pixels to disk and never reports keystroke content;
/// - never reports file names inside protected roots;
/// - never locks, sleeps or wakes the machine, and never touches `pmset`/`tccutil`.

// MARK: - Entry point (main.swift top-level code)

let exitCode: Int32
do {
    exitCode = try ProbeCLI.run(
        arguments: Array(CommandLine.arguments.dropFirst()),
        commandLine: CommandLine.arguments
    )
} catch let error as ProbeCLIError {
    FileHandle.standardError.write(Data(("codebridge-probe: \(error.description)\n").utf8))
    exitCode = error.exitCode
} catch {
    FileHandle.standardError.write(Data(("codebridge-probe: unexpected error \(error)\n").utf8))
    exitCode = 3
}
exit(exitCode)
