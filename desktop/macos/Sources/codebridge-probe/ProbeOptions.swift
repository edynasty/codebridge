import CodeBridgeIPC
import Foundation

struct ProbeOptions {
    var probe: String = "list"
    var pretty: Bool = false
    var outputPath: String?
    var duration: Double?
    var roots: [String] = ProbeOptions.defaultRoots
    var onlyTools: [String]?
    var handshakeRole: String?
    var showHelp: Bool = false
    var commandLine: [String] = []

    static let defaultRoots = ["Desktop", "Documents", "Downloads", "/tmp"]
    static let knownProbes: [String] = [
        "permissions", "signing", "native-host", "harness",
        "lock-state", "input-monitor", "files-folders", "all", "list",
    ]
    static let defaultObservationSeconds: Double = 5
    static let maxObservationSeconds: Double = 300
}

enum ProbeOptionsParser {
    static func parse(arguments: [String], commandLine: [String]) throws -> ProbeOptions {
        var options = ProbeOptions()
        options.commandLine = commandLine
        var index = 0
        var sawProbe = false

        while index < arguments.count {
            let argument = arguments[index]
            switch argument {
            case "--help", "-h":
                options.showHelp = true
                index += 1
            case "--json":
                index += 1
            case "--pretty":
                options.pretty = true
                index += 1
            case "--output":
                guard let value = value(after: index, in: arguments) else {
                    throw ProbeCLIError.usage("--output requires a path")
                }
                options.outputPath = value
                index += 2
            case "--duration":
                guard let value = value(after: index, in: arguments), let seconds = Double(value) else {
                    throw ProbeCLIError.usage("--duration requires a number of seconds")
                }
                guard seconds >= 0, seconds <= ProbeOptions.maxObservationSeconds else {
                    throw ProbeCLIError.usage("--duration must be between 0 and \(Int(ProbeOptions.maxObservationSeconds))")
                }
                options.duration = seconds
                index += 2
            case "--roots":
                guard let value = value(after: index, in: arguments) else {
                    throw ProbeCLIError.usage("--roots requires a comma-separated list")
                }
                options.roots = value.split(separator: ",").map { String($0).trimmingCharacters(in: .whitespaces) }
                    .filter { !$0.isEmpty }
                index += 2
            case "--only":
                guard let value = value(after: index, in: arguments) else {
                    throw ProbeCLIError.usage("--only requires a comma-separated list")
                }
                options.onlyTools = value.split(separator: ",").map { String($0).trimmingCharacters(in: .whitespaces) }
                    .filter { !$0.isEmpty }
                index += 2
            case "--handshake-role":
                guard let value = value(after: index, in: arguments) else {
                    throw ProbeCLIError.usage("--handshake-role requires a role")
                }
                let allowed = [HostIPCRole.app, HostIPCRole.diagnostics, HostIPCRole.harness]
                guard allowed.contains(value) else {
                    throw ProbeCLIError.usage("--handshake-role must be one of \(allowed.joined(separator: ", "))")
                }
                options.handshakeRole = value
                index += 2
            default:
                guard !argument.hasPrefix("--") else {
                    throw ProbeCLIError.usage("unknown option \(argument)")
                }
                guard !sawProbe else {
                    throw ProbeCLIError.usage("unexpected argument \(argument)")
                }
                options.probe = argument
                sawProbe = true
                index += 1
            }
        }

        if !ProbeOptions.knownProbes.contains(options.probe) {
            throw ProbeCLIError.usage(
                "unknown probe \(options.probe); expected one of \(ProbeOptions.knownProbes.joined(separator: ", "))"
            )
        }
        return options
    }

    private static func value(after index: Int, in arguments: [String]) -> String? {
        let next = index + 1
        guard next < arguments.count else { return nil }
        return arguments[next]
    }
}
