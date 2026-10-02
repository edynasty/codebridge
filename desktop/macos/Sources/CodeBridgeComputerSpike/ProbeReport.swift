import CodeBridgeIPC
import Foundation

/// Phase 0 probe report schema (`codebridge.phase0.probe.v1`).
///
/// The same struct is produced by the standalone `codebridge-probe` harness and decoded by the app
/// when it runs the harness as its child, so app-attributed and daemon-attributed runs are directly
/// comparable in the evidence.
///
/// Privacy invariants encoded here:
/// - never report file names inside protected roots (only `exists` / `readable` / `errno` / `entry_count`);
/// - never persist or embed pixels (only dimensions and a digest of the in-memory buffer);
/// - never report keystroke content (only counts and conservative origin classification).
public struct Phase0ProbeReport: Codable, Sendable {
    public static let schemaVersion = "codebridge.phase0.probe.v1"

    public var schema: String
    public var probe: String
    public var command_line: [String]
    public var started_at: String
    public var ended_at: String
    public var duration_seconds: Double
    public var host: ProbeHostInfo
    public var launch: ProbeLaunchIdentity
    public var permissions: PermissionsSection?
    public var signing: SigningExtraSection?
    public var host_tools: HostToolsSection?
    public var lock_state: LockStateSection?
    public var input_monitor: InputMonitorSection?
    /// Files & Folders can be exercised without any Computer Use API call.
    public var files_folders: FilesFoldersProbeReport?
    public var findings: ProbeFindings
    public var privacy: String

    public init(
        probe: String,
        commandLine: [String],
        startedAt: String,
        endedAt: String,
        durationSeconds: Double,
        host: ProbeHostInfo,
        launch: ProbeLaunchIdentity,
        permissions: PermissionsSection? = nil,
        signing: SigningExtraSection? = nil,
        hostTools: HostToolsSection? = nil,
        lockState: LockStateSection? = nil,
        inputMonitor: InputMonitorSection? = nil,
        filesFolders: FilesFoldersProbeReport? = nil,
        findings: ProbeFindings
    ) {
        self.schema = Phase0ProbeReport.schemaVersion
        self.probe = probe
        self.command_line = commandLine
        self.started_at = startedAt
        self.ended_at = endedAt
        self.duration_seconds = durationSeconds
        self.host = host
        self.launch = launch
        self.permissions = permissions
        self.signing = signing
        self.host_tools = hostTools
        self.lock_state = lockState
        self.input_monitor = inputMonitor
        self.files_folders = filesFolders
        self.findings = findings
        self.privacy = "no file names inside protected roots, no pixels persisted, no keystroke content, no input injected"
    }
}

public struct ProbeHostInfo: Codable, Sendable {
    public var os_version: String
    public var os_build: String
    public var architecture: String
    public var model_identifier: String?
    public var user_uid: uid_t
    public var gui_session_available: Bool
    public var launchd_domain: String?

    public init(
        os_version: String,
        os_build: String,
        architecture: String,
        model_identifier: String?,
        user_uid: uid_t,
        gui_session_available: Bool,
        launchd_domain: String?
    ) {
        self.os_version = os_version
        self.os_build = os_build
        self.architecture = architecture
        self.model_identifier = model_identifier
        self.user_uid = user_uid
        self.gui_session_available = gui_session_available
        self.launchd_domain = launchd_domain
    }
}

public struct ProbeLaunchIdentity: Codable, Sendable {
    public var pid: Int32
    public var parent_pid: Int32
    public var executable_path: String?
    public var parent_executable_path: String?
    public var launch_context: String
    public var bundle_identifier: String?
    public var bundle_path: String?
    public var signed: Bool
    public var signing_identifier: String?
    public var team_identifier: String?
    public var cdhash: String?
    public var adhoc_signed: Bool
    public var signature_valid: Bool
    public var signature_error: String?
    public var entitlements: [String: JSONValue]?

    public init(identity: ProcessIdentity, signing: SigningIdentity?) {
        self.pid = identity.pid
        self.parent_pid = identity.parentPID
        self.executable_path = identity.executablePath
        self.parent_executable_path = identity.parentExecutablePath
        self.launch_context = identity.launchContext
        self.bundle_identifier = Bundle.main.bundleIdentifier
        self.bundle_path = Bundle.main.bundlePath
        self.signed = signing?.signingIdentifier != nil
        self.signing_identifier = signing?.signingIdentifier
        self.team_identifier = signing?.teamIdentifier
        self.cdhash = signing?.cdhashHex
        self.adhoc_signed = signing?.isAdHoc ?? false
        self.signature_valid = signing?.isValid ?? false
        self.signature_error = signing == nil ? "no code signature available (unsigned or unreadable)" : nil
        self.entitlements = signing?.entitlements
    }
}

// MARK: - Permissions

public struct PermissionsSection: Codable, Sendable {
    public var screen_capture: ScreenCaptureProbeReport
    public var accessibility: AccessibilityProbeReport
    public var files_folders: FilesFoldersProbeReport
    public var input_monitoring_preflight: InputMonitoringPreflightReport

    public init(
        screen_capture: ScreenCaptureProbeReport,
        accessibility: AccessibilityProbeReport,
        files_folders: FilesFoldersProbeReport,
        input_monitoring_preflight: InputMonitoringPreflightReport
    ) {
        self.screen_capture = screen_capture
        self.accessibility = accessibility
        self.files_folders = files_folders
        self.input_monitoring_preflight = input_monitoring_preflight
    }
}

public struct ScreenCaptureProbeReport: Codable, Sendable {
    public var preflight_granted: Bool
    public var shareable_content_call: String
    public var shareable_content_error: String?
    public var display_count: Int?
    public var capture_call: String
    public var capture_error: String?
    public var captured_width: Int?
    public var captured_height: Int?
    public var captured_sha256: String?
    public var pixels_persisted: Bool

    public init(
        preflight_granted: Bool,
        shareable_content_call: String,
        shareable_content_error: String?,
        display_count: Int?,
        capture_call: String,
        capture_error: String?,
        captured_width: Int?,
        captured_height: Int?,
        captured_sha256: String?,
        pixels_persisted: Bool
    ) {
        self.preflight_granted = preflight_granted
        self.shareable_content_call = shareable_content_call
        self.shareable_content_error = shareable_content_error
        self.display_count = display_count
        self.capture_call = capture_call
        self.capture_error = capture_error
        self.captured_width = captured_width
        self.captured_height = captured_height
        self.captured_sha256 = captured_sha256
        self.pixels_persisted = pixels_persisted
    }
}

public struct AccessibilityProbeReport: Codable, Sendable {
    public var trusted: Bool
    public var focused_application_call: String
    public var focused_application_error: String?
    public var api_error_code: Int32?

    public init(
        trusted: Bool,
        focused_application_call: String,
        focused_application_error: String?,
        api_error_code: Int32?
    ) {
        self.trusted = trusted
        self.focused_application_call = focused_application_call
        self.focused_application_error = focused_application_error
        self.api_error_code = api_error_code
    }
}

public struct ProtectedRootProbeReport: Codable, Sendable {
    public var path: String
    public var exists: Bool
    public var readable: Bool
    public var errno: Int32?
    public var entry_count: Int?
    /// Set when the probe did not return in time: the kernel blocked the access with no TCC decision.
    public var blocked_seconds: Double?
    public var note: String?

    public init(
        path: String,
        exists: Bool,
        readable: Bool,
        errno: Int32?,
        entry_count: Int?,
        blocked_seconds: Double? = nil,
        note: String? = nil
    ) {
        self.path = path
        self.exists = exists
        self.readable = readable
        self.errno = errno
        self.entry_count = entry_count
        self.blocked_seconds = blocked_seconds
        self.note = note
    }
}

public struct FilesFoldersProbeReport: Codable, Sendable {
    public var roots: [ProtectedRootProbeReport]
    public var note: String

    public init(roots: [ProtectedRootProbeReport], note: String) {
        self.roots = roots
        self.note = note
    }
}

public struct InputMonitoringPreflightReport: Codable, Sendable {
    public var preflight_listen_access: Bool
    public var preflight_post_access: Bool
    public var secure_input_enabled: Bool

    public init(preflight_listen_access: Bool, preflight_post_access: Bool, secure_input_enabled: Bool) {
        self.preflight_listen_access = preflight_listen_access
        self.preflight_post_access = preflight_post_access
        self.secure_input_enabled = secure_input_enabled
    }
}

public struct InputMonitorSection: Codable, Sendable {
    public var preflight: InputMonitoringPreflightReport
    public var tap_created: Bool
    public var tap_error: String?
    public var tap_disabled_count: Int
    public var observation_seconds: Double
    public var event_counts_by_origin: [String: Int]
    public var preemption_decisions: [String: Int]
    public var monitor_state: String
    public var input_available: Bool
    public var unavailability_reason: String?
    public var injected_events: Int
    public var hardware_events_observed: Int
    public var delivery_proof: String
    public var note: String

    public init(
        preflight: InputMonitoringPreflightReport,
        tap_created: Bool,
        tap_error: String?,
        tap_disabled_count: Int,
        observation_seconds: Double,
        event_counts_by_origin: [String: Int],
        preemption_decisions: [String: Int],
        monitor_state: String,
        input_available: Bool,
        unavailability_reason: String?,
        injected_events: Int,
        hardware_events_observed: Int,
        delivery_proof: String,
        note: String
    ) {
        self.preflight = preflight
        self.tap_created = tap_created
        self.tap_error = tap_error
        self.tap_disabled_count = tap_disabled_count
        self.observation_seconds = observation_seconds
        self.event_counts_by_origin = event_counts_by_origin
        self.preemption_decisions = preemption_decisions
        self.monitor_state = monitor_state
        self.input_available = input_available
        self.unavailability_reason = unavailability_reason
        self.injected_events = injected_events
        self.hardware_events_observed = hardware_events_observed
        self.delivery_proof = delivery_proof
        self.note = note
    }
}

// MARK: - Signing

public struct SigningExtraSection: Codable, Sendable {
    public var parent_signed: Bool
    public var parent_signing_identifier: String?
    public var parent_team_identifier: String?
    public var parent_signature_error: String?
    public var team_identifier_configured: Bool
    public var tcc_attribution_note: String

    public init(
        parent_signed: Bool,
        parent_signing_identifier: String?,
        parent_team_identifier: String?,
        parent_signature_error: String?,
        team_identifier_configured: Bool,
        tcc_attribution_note: String
    ) {
        self.parent_signed = parent_signed
        self.parent_signing_identifier = parent_signing_identifier
        self.parent_team_identifier = parent_team_identifier
        self.parent_signature_error = parent_signature_error
        self.team_identifier_configured = team_identifier_configured
        self.tcc_attribution_note = tcc_attribution_note
    }
}

// MARK: - Native host

public struct HostToolProbeReport: Codable, Sendable {
    public var name: String
    public var resolved_path: String?
    public var available: Bool
    public var version: String?
    public var detail: String?

    public init(
        name: String,
        resolved_path: String?,
        available: Bool,
        version: String?,
        detail: String?
    ) {
        self.name = name
        self.resolved_path = resolved_path
        self.available = available
        self.version = version
        self.detail = detail
    }
}

public struct HostToolsSection: Codable, Sendable {
    public var tools: [HostToolProbeReport]
    public var launch_context: String
    public var note: String

    public init(tools: [HostToolProbeReport], launch_context: String, note: String) {
        self.tools = tools
        self.launch_context = launch_context
        self.note = note
    }
}

// MARK: - Lock state

public struct LockStateSnapshot: Codable, Sendable {
    public var on_console: Bool?
    public var screen_locked: Bool?
    public var session_dictionary_keys: [String]
    public var note: String

    public init(
        on_console: Bool?,
        screen_locked: Bool?,
        session_dictionary_keys: [String],
        note: String
    ) {
        self.on_console = on_console
        self.screen_locked = screen_locked
        self.session_dictionary_keys = session_dictionary_keys
        self.note = note
    }
}

public struct LockStateEvent: Codable, Sendable {
    public var at: String
    public var kind: String
    public var source: String
    public var detail: String?

    public init(at: String, kind: String, source: String, detail: String?) {
        self.at = at
        self.kind = kind
        self.source = source
        self.detail = detail
    }
}

public struct LockStateSection: Codable, Sendable {
    public var initial: LockStateSnapshot
    public var final: LockStateSnapshot
    public var events: [LockStateEvent]
    public var observation_seconds: Double
    public var os_transitions_observed: [String]
    public var model_transitions_applied: [ControllerTransitionSummary]
    public var queue_replay_blocked: Bool
    public var evidence_class: String
    public var note: String

    public init(
        initial: LockStateSnapshot,
        final: LockStateSnapshot,
        events: [LockStateEvent],
        observation_seconds: Double,
        os_transitions_observed: [String],
        model_transitions_applied: [ControllerTransitionSummary],
        queue_replay_blocked: Bool,
        evidence_class: String,
        note: String
    ) {
        self.initial = initial
        self.final = final
        self.events = events
        self.observation_seconds = observation_seconds
        self.os_transitions_observed = os_transitions_observed
        self.model_transitions_applied = model_transitions_applied
        self.queue_replay_blocked = queue_replay_blocked
        self.evidence_class = evidence_class
        self.note = note
    }
}

/// Codable projection of `ControllerTransition` (the model type itself is not `Codable`).
public struct ControllerTransitionSummary: Codable, Sendable {
    public var reason: String
    public var previous_controller: String
    public var controller: String
    public var controller_epoch: Int
    public var discarded_actions: Int
    public var geometry_generation: Int
    public var requires_human_resume: Bool
    public var state: String

    public init(transition: ControllerTransition) {
        self.reason = transition.reason.rawValue
        self.previous_controller = transition.previousController.rawValue
        self.controller = transition.controller.rawValue
        self.controller_epoch = transition.controllerEpoch
        self.discarded_actions = transition.discardedActions
        self.geometry_generation = transition.geometryGeneration
        self.requires_human_resume = transition.requiresHumanResume
        self.state = transition.state.rawValue
    }
}

// MARK: - Findings

public struct ProbeFindings: Codable, Sendable {
    public var observed: [String]
    public var untested: [String]
    public var blocked: [String]

    public init(observed: [String] = [], untested: [String] = [], blocked: [String] = []) {
        self.observed = observed
        self.untested = untested
        self.blocked = blocked
    }
}

// MARK: - Rendering

public enum ProbeReportWriter {
    public static func encode<T: Encodable>(_ value: T, pretty: Bool) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = pretty
            ? [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
            : [.sortedKeys, .withoutEscapingSlashes]
        return try encoder.encode(value)
    }

    public static func emit<T: Encodable>(_ value: T, pretty: Bool, outputPath: String?) throws {
        let data = try encode(value, pretty: pretty)
        var stdoutData = data
        stdoutData.append(0x0a)
        FileHandle.standardOutput.write(stdoutData)
        if let path = outputPath {
            let url = URL(fileURLWithPath: path)
            try data.write(to: url, options: .atomic)
        }
    }

    public static func timestamp(_ date: Date = Date()) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }
}
