import AppKit
import ApplicationServices
import CoreGraphics
import CodeBridgeIPC
import CodeBridgeNativeProbe
import Foundation
import Security

/// Phase 0B acceptance entry points. Permission requests are explicit App-only opt-in;
/// observation probes never request grants, inject input or spawn the daemon.
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

    private struct PermissionRequestReceipt: Encodable {
        let schema = "codebridge.phase0b.permission_request.v1"
        let pid: Int32
        let bundlePath: String
        let signingIdentifier: String?
        let teamIdentifier: String?
        let displayName: String?
        let permission: String
        var stage: String
        var screenRequestReturned: Bool?
        var accessibilityPromptTrusted: Bool?
        var listenRequestReturned: Bool?
        let note = "Request returns are not positive API acceptance. Human authorization and App restart are required."
    }

    /// Called on the launched App's main thread. No child/harness requests any permission.
    static func requestComputerPermission(arguments: [String]) throws {
        try requireSignedBundle()
        guard let index = arguments.firstIndex(of: "--phase0b-request-permission"),
              index + 1 < arguments.count,
              ["screen", "accessibility", "input"].contains(arguments[index + 1])
        else { throw ProbeCLIError.usage("--phase0b-request-permission requires screen|accessibility|input") }
        let permission = arguments[index + 1]
        let outputPath: String?
        if let index = arguments.firstIndex(of: "--output") {
            guard index + 1 < arguments.count else { throw ProbeCLIError.usage("--output requires a path") }
            outputPath = arguments[index + 1]
        } else {
            outputPath = nil
        }
        let identity = PeerIdentity.signingIdentity(pid: getpid())
        var receipt = PermissionRequestReceipt(
            pid: getpid(), bundlePath: Bundle.main.bundlePath,
            signingIdentifier: identity?.signingIdentifier, teamIdentifier: identity?.teamIdentifier,
            displayName: Bundle.main.object(forInfoDictionaryKey: "CFBundleDisplayName") as? String,
            permission: permission, stage: "requesting_" + permission
        )
        func record() throws {
            let encoder = JSONEncoder()
            encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
            let data = try encoder.encode(receipt)
            if let outputPath {
                try data.write(to: URL(fileURLWithPath: outputPath), options: .atomic)
            }
            FileHandle.standardOutput.write(data)
            FileHandle.standardOutput.write(Data("\n".utf8))
        }
        NSApplication.shared.activate(ignoringOtherApps: true)
        try record()
        switch permission {
        case "screen":
            receipt.screenRequestReturned = CGRequestScreenCaptureAccess()
        case "accessibility":
            receipt.accessibilityPromptTrusted = AXIsProcessTrustedWithOptions(
                [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
            )
        case "input":
            receipt.listenRequestReturned = CGRequestListenEventAccess()
        default:
            throw ProbeCLIError.usage("unknown permission")
        }
        receipt.stage = "request_returned_awaiting_human_authorization_and_restart"
        try record()
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
