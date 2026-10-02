import Carbon.HIToolbox
import CodeBridgeComputerSpike
import CoreGraphics
import Foundation

enum ProbeSelection: Hashable {
    case permissions
    case signing
    case nativeHost
    case lockState
    case inputMonitor
    case filesFolders
}

enum ProbeRunner {
    static func run(options: ProbeOptions) throws -> Phase0ProbeReport {
        let startedAt = ProbeReportWriter.timestamp()
        let startedDate = Date()
        let host = HostEnvironment.hostInfo()
        let launch = HostEnvironment.launchIdentity()
        let selection = selection(for: options.probe)

        var permissions: PermissionsSection?
        var signing: SigningExtraSection?
        var hostTools: HostToolsSection?
        var lockState: LockStateSection?
        var inputMonitor: InputMonitorSection?
        var filesFolders: FilesFoldersProbeReport?
        var observed: [String] = []
        var untested: [String] = []
        var blocked: [String] = []

        if options.probe == "list" {
            observed = ProbeOptions.knownProbes.map { "probe: \($0)" }
            untested = []
            blocked = []
        }

        if selection.contains(.permissions) {
            let screenCapture = ScreenCaptureProbe.run()
            let accessibility = AccessibilityProbe.run()
            let filesFolders = FilesFoldersProbe.run(rootNames: options.roots)
            let preflight = InputMonitoringPreflightReport(
                preflight_listen_access: CGPreflightListenEventAccess(),
                preflight_post_access: CGPreflightPostEventAccess(),
                secure_input_enabled: IsSecureEventInputEnabled()
            )
            permissions = PermissionsSection(
                screen_capture: screenCapture,
                accessibility: accessibility,
                files_folders: filesFolders,
                input_monitoring_preflight: preflight
            )
            observed.append(
                "screen_capture: preflight=\(screenCapture.preflight_granted) "
                    + "shareable_content=\(screenCapture.shareable_content_call)"
                    + errorSuffix(screenCapture.shareable_content_error)
                    + " capture=\(screenCapture.capture_call)"
                    + errorSuffix(screenCapture.capture_error)
                    + " pixels_persisted=\(screenCapture.pixels_persisted)"
            )
            observed.append(
                "accessibility: trusted=\(accessibility.trusted) "
                    + "focused_application=\(accessibility.focused_application_call) "
                    + "ax_error=\(accessibility.api_error_code.map { String($0) } ?? "none")"
            )
            observed.append(
                "input_monitoring_preflight: listen=\(preflight.preflight_listen_access) "
                    + "post=\(preflight.preflight_post_access) secure_input=\(preflight.secure_input_enabled)"
            )
            let roots = filesFolders.roots
                .map { root in
                    "\(root.path): exists=\(root.exists) readable=\(root.readable) "
                        + "errno=\(root.errno.map { String($0) } ?? "none") "
                        + "entries=\(root.entry_count.map { String($0) } ?? "none")"
                }
                .joined(separator: "; ")
            observed.append("protected_roots(read-only, names not reported): \(roots)")
            untested.append(
                "real TCC grant state for a signed CodeBridge.app bundle: this run has "
                    + "launch_context=\(launch.launch_context) signed=\(launch.signed) "
                    + "identity=\(launch.signing_identifier ?? "none")"
            )
            untested.append(
                "TCC attribution comparison across launch contexts (app child / codebridged child / standalone)"
            )
        }

        if selection.contains(.signing) {
            let extra = HostEnvironment.signingExtra()
            signing = extra
            observed.append(
                "self: pid=\(launch.pid) ppid=\(launch.parent_pid) context=\(launch.launch_context) "
                    + "path=\(launch.executable_path ?? "unknown") signed=\(launch.signed) "
                    + "identifier=\(launch.signing_identifier ?? "none") "
                    + "team=\(launch.team_identifier ?? "none") adhoc=\(launch.adhoc_signed) "
                    + "valid=\(launch.signature_valid) cdhash=\(launch.cdhash ?? "none")"
            )
            observed.append(
                "parent: path=\(launch.parent_executable_path ?? "unknown") "
                    + "signed=\(extra.parent_signed) identifier=\(extra.parent_signing_identifier ?? "none") "
                    + "team=\(extra.parent_team_identifier ?? "none")"
            )
            untested.append(
                "expected Team ID / daemon signing identity presence "
                    + "(configured=\(extra.team_identifier_configured))"
            )
            if !extra.team_identifier_configured {
                blocked.append(
                    "no CODEBRIDGE_EXPECT_TEAM_ID / daemon signing identity configured in this environment"
                )
            }
            if !launch.signed {
                blocked.append(
                    "this process has no code-signing identity, so OS permission grants cannot be attributed to a "
                        + "signed bundle; codebridged rejects role=app without a verified live peer signature"
                )
            }
        }

        if selection.contains(.nativeHost) {
            let tools = HostToolsProbe.run(only: options.onlyTools)
            hostTools = tools
            let summary = tools.tools
                .map { tool in
                    "\(tool.name)=\(tool.available ? (tool.resolved_path ?? "available") : "missing")"
                        + (tool.version.map { " (\($0))" } ?? "")
                        + (tool.detail.map { " [\($0)]" } ?? "")
                }
                .joined(separator: "; ")
            observed.append("host_tools from \(tools.launch_context): \(summary)")
            untested.append(
                "authoritative \"works from codebridged\" evidence: host.native_host_smoke must run inside the daemon "
                    + "process (this report is from the \(tools.launch_context) process tree)"
            )
        }

        if selection.contains(.lockState) {
            let duration = options.duration ?? ProbeOptions.defaultObservationSeconds
            let section = LockStateProbe.run(duration: duration)
            lockState = section
            if section.os_transitions_observed.isEmpty {
                observed.append(
                    "no lock/sleep/session-switch transition occurred during the \(Int(duration))s observation window"
                )
            } else {
                observed.append("observed OS transitions: \(section.os_transitions_observed.joined(separator: ", "))")
            }
            observed.append(
                "state-machine spike: applied \(section.model_transitions_applied.count) suspension transition(s); "
                    + "queue_replay_blocked=\(section.queue_replay_blocked)"
            )
            if section.os_transitions_observed.isEmpty {
                untested.append(
                    "real lock / wake / fast-user-switch transitions on this host: run `codebridge-probe lock-state "
                        + "--json --duration 120` and lock the screen manually; the probe never triggers lock or sleep"
                )
            }
            blocked.append(
                "no OS evidence about queued CGEvents after unlock is claimed: discard is proven only by the "
                    + "arbiter state machine, and ScreenCaptureKit/CGEvent behavior during lock requires a "
                    + "signed, granted build"
            )
        }

        if selection.contains(.inputMonitor) {
            let duration = options.duration ?? ProbeOptions.defaultObservationSeconds
            let section = InputMonitorProbe.run(duration: duration)
            inputMonitor = section
            observed.append(
                "input_monitor: tap_created=\(section.tap_created) monitor_state=\(section.monitor_state) "
                    + "input_available=\(section.input_available) disabled_events=\(section.tap_disabled_count) "
                    + "hardware_events=\(section.hardware_events_observed) delivery_proof=\(section.delivery_proof) "
                    + "origins=\(section.event_counts_by_origin)"
            )
            observed.append("input_monitor decisions: \(section.preemption_decisions)")
            untested.append(
                "real hardware-input preemption with Input Monitoring granted to a signed bundle, and preemption "
                    + "against synthetic input from another Accessibility-permitted process"
            )
            if !section.input_available {
                blocked.append(
                    "input monitor is not verified usable on this host right now "
                        + "(\(section.unavailability_reason ?? "unknown")): per architecture §11.6 computer.input must be "
                        + "reported unavailable (fail closed) until hardware-input preemption is observed on a signed, "
                        + "granted build"
                )
            }
        }
        if selection.contains(.filesFolders) {
            filesFolders = FilesFoldersProbe.run(rootNames: options.roots)
            observed.append("Files & Folders only: read-only root probe, 5s deadline per root; no Computer Use API called")
            untested.append("signed protected-root grant persistence across restart/rebuild/update")
        }


        let endedAt = ProbeReportWriter.timestamp()
        return Phase0ProbeReport(
            probe: options.probe,
            commandLine: options.commandLine,
            startedAt: startedAt,
            endedAt: endedAt,
            durationSeconds: Date().timeIntervalSince(startedDate),
            host: host,
            launch: launch,
            permissions: permissions,
            signing: signing,
            hostTools: hostTools,
            lockState: lockState,
            inputMonitor: inputMonitor,
            filesFolders: filesFolders,
            findings: ProbeFindings(observed: observed, untested: untested, blocked: blocked)
        )
    }

    static func selection(for probe: String) -> Set<ProbeSelection> {
        switch probe {
        case "permissions":
            return [.permissions]
        case "signing":
            return [.signing]
        case "native-host":
            return [.nativeHost]
        case "harness":
            return [.permissions, .signing]
        case "lock-state":
            return [.lockState]
        case "input-monitor":
            return [.inputMonitor]
        case "files-folders":
            return [.filesFolders]
        case "all":
            return [.permissions, .signing, .nativeHost]
        default:
            return []
        }
    }

    private static func errorSuffix(_ error: String?) -> String {
        guard let error = error else { return "" }
        return " error=\(error)"
    }
}
