import CodeBridgeIPC
import CodeBridgeNativeProbe
import Foundation
import Security

/// Phase 0B acceptance entry points. No grant request, input injection or daemon spawn.
enum Phase0BAcceptance {
    static func requireSignedBundle() throws {
        let info = Bundle.main.infoDictionary ?? [:]
        guard info["CodeBridgeBuildKind"] as? String == "signed",
              info["CodeBridgeUnsignedDevBuild"] as? Bool == false,
              let identity = PeerIdentity.signingIdentity(pid: getpid()),
              let team = identity.teamIdentifier, !team.isEmpty,
              team == info["CodeBridgeExpectedTeamIdentifier"] as? String,
              !identity.isAdHoc,
              let requirement = PeerRequirement.make(identifier: "com.codebridge.app", teamIdentifier: team)
        else { throw ProbeCLIError.internalFailure("SIGNED_TCC_BLOCKED_EXTERNAL: genuine signed bundle required") }

        var code: SecCode?
        guard SecCodeCopySelf([], &code) == errSecSuccess, let code = code,
              SecCodeCheckValidity(code, [], requirement) == errSecSuccess
        else { throw ProbeCLIError.internalFailure("SIGNED_TCC_BLOCKED_EXTERNAL: live Apple-anchored App verification failed") }

        for (name, identifier) in [("codebridged", "com.codebridge.daemon"),
                                   ("codebridge-probe", "com.codebridge.probe")] {
            let url = Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/" + name)
            var nested: SecStaticCode?
            guard let expected = PeerRequirement.make(identifier: identifier, teamIdentifier: team),
                  SecStaticCodeCreateWithPath(url as CFURL, [], &nested) == errSecSuccess,
                  let nested = nested,
                  SecStaticCodeCheckValidity(nested, [], expected) == errSecSuccess
            else { throw ProbeCLIError.internalFailure("SIGNED_TCC_BLOCKED_EXTERNAL: nested identity verification failed: \(identifier)") }
        }
    }

    static func run(arguments: [String]) throws -> Int32 {
        if let index = arguments.firstIndex(of: "--phase0b-probe") {
            let probeArguments = Array(arguments.dropFirst(index + 1))
            guard let probe = probeArguments.first else { throw ProbeCLIError.usage("--phase0b-probe requires a probe name") }
            // Identity/list reports are harmless without a certificate. OS acceptance is not.
            if probe != "signing" && probe != "list" { try requireSignedBundle() }
            return try ProbeCLI.run(arguments: probeArguments, commandLine: arguments)
        }
        guard let index = arguments.firstIndex(of: "--phase0b-service"), index + 1 < arguments.count
        else { throw ProbeCLIError.usage("--phase0b-service requires status|register|unregister") }
        let service = LaunchAgentController()
        switch arguments[index + 1] {
        case "status": break
        case "register": try service.register()
        case "unregister":
            try requireSignedBundle()
            try service.unregister()
        default: throw ProbeCLIError.usage("--phase0b-service requires status|register|unregister")
        }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        FileHandle.standardOutput.write(try encoder.encode(service.daemonJobInfo()))
        FileHandle.standardOutput.write(Data("\n".utf8))
        return 0
    }
}
