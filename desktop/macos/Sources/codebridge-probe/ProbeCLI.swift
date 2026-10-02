import CodeBridgeComputerSpike
import Foundation

public enum ProbeCLIError: Error, CustomStringConvertible {
    case usage(String)
    case internalFailure(String)

    public var exitCode: Int32 {
        switch self {
        case .usage: return 2
        case .internalFailure: return 3
        }
    }

    public var description: String {
        switch self {
        case .usage(let detail): return detail
        case .internalFailure(let detail): return "internal failure: \(detail)"
        }
    }
}

public enum ProbeCLI {
    public static func run(arguments: [String], commandLine: [String]) throws -> Int32 {
        let options: ProbeOptions
        do {
            options = try ProbeOptionsParser.parse(arguments: arguments, commandLine: commandLine)
        } catch let error as ProbeCLIError {
            FileHandle.standardError.write(Data((usageText + "\n").utf8))
            throw error
        }

        if options.showHelp {
            FileHandle.standardError.write(Data((usageText + "\n").utf8))
            return 0
        }

        if let role = options.handshakeRole {
            return try HostIPCHandshakeProbe.run(
                role: role,
                pretty: options.pretty,
                outputPath: options.outputPath
            )
        }

        let report = try ProbeRunner.run(options: options)
        try ProbeReportWriter.emit(report, pretty: options.pretty, outputPath: options.outputPath)
        return 0
    }

    static let usageText = """
    usage: codebridge-probe <probe> [options]

    probes:
      permissions     ScreenCaptureKit preflight + real call, Accessibility API, Input Monitoring
                      preflight, protected-root read probes, signing/process identity
      signing         code-signing identity of this process and its parent, launch context
      native-host     availability + version of shell/git/docker/ssh/kubectl in this process tree
      harness         alias for permissions + signing (used for harness-child attribution runs)
      lock-state      observe screen lock / sleep / fast-user-switch notifications for --duration
      input-monitor   create a listen-only event tap and classify input origin for --duration
      files-folders   Files & Folders only; bounded read without ScreenCapture/AX/event-tap calls
      all             permissions + signing + native-host (no observation window)
      list            list available probes

    options:
      --json                 emit one JSON object on stdout (default; part of the frozen argv shape)
      --pretty               indent the JSON
      --output <path>        also write the JSON report to <path>
      --duration <seconds>   observation window for lock-state / input-monitor
      --roots <a,b>          protected roots to probe (names are relative to $HOME)
      --only <a,b>           limit native-host checks
      --handshake-role <r>   skip probing and run a Host IPC handshake as app|diagnostics|harness
                             against the running daemon (no computer/approval method is called)

    Safety: no TCC request, no input injection, no pixel persistence, no file names in protected
    roots, no lock/sleep trigger, no tccutil.
    """
}
