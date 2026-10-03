import AppKit
import CodeBridgeIPC

/// Menu-bar UI for the Phase 0 host adapter.
///
/// Deliberately contains **no** Computer Use implementation: no capture, no `CGEvent` injection, no
/// approval flow, no InputArbiter. It registers the LaunchAgent, exercises the Host IPC handshake
/// and runs permission probes.
final class MenuBarController: NSObject, NSMenuDelegate {
    private let statusItem: NSStatusItem
    private let launchAgent = LaunchAgentController()
    private let installer = DaemonInstaller.shared
    private let worker = DispatchQueue(label: "com.codebridge.app.worker", qos: .userInitiated)

    override init() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        super.init()
        statusItem.button?.title = "CB"
        statusItem.menu = buildMenu()
    }

    func shutdown() {
        NSStatusBar.system.removeStatusItem(statusItem)
    }

    func install() {
        NSApp.setActivationPolicy(.accessory)
        refreshStatusItem()
    }

    // MARK: - Menu

    private func buildMenu() -> NSMenu {
        let menu = NSMenu()
        menu.delegate = self

        menu.addItem(withTitle: "Host IPC: handshake as app", action: #selector(handshakeAsApp), keyEquivalent: "")
        menu.addItem(withTitle: "Host IPC: handshake as diagnostics", action: #selector(handshakeAsDiagnostics), keyEquivalent: "")
        menu.addItem(NSMenuItem.separator())
        menu.addItem(withTitle: "Daemon: install external (production)", action: #selector(installExternalDaemon), keyEquivalent: "")
        menu.addItem(withTitle: "Daemon: uninstall external", action: #selector(uninstallExternalDaemon), keyEquivalent: "")
        menu.addItem(NSMenuItem.separator())
        menu.addItem(withTitle: "Probe: permissions (CodeBridge.app child)", action: #selector(probeAsAppChild), keyEquivalent: "")
        menu.addItem(withTitle: "Probe: permissions (codebridged child)", action: #selector(probeViaDaemon), keyEquivalent: "")
        menu.addItem(withTitle: "Probe: input monitor (CodeBridge.app child)", action: #selector(probeInputMonitor), keyEquivalent: "")
        menu.addItem(NSMenuItem.separator())
        menu.addItem(withTitle: "Show my signing identity", action: #selector(showSigningIdentity), keyEquivalent: "")
        menu.addItem(withTitle: "Show bundle paths", action: #selector(showBundlePaths), keyEquivalent: "")
        menu.addItem(NSMenuItem.separator())
        menu.addItem(withTitle: "Quit CodeBridge", action: #selector(quit), keyEquivalent: "q")

        for item in menu.items where item.action != nil {
            item.target = self
        }
        return menu
    }

    func menuWillOpen(_ menu: NSMenu) {
        refreshStatusItem()
    }

    private func refreshStatusItem() {
        let sign = PeerIdentity.signingIdentity(pid: getpid())?.teamIdentifier
        statusItem.button?.title = sign == nil ? "CB (unsigned)" : "CB"
        var lines: [String] = ["CodeBridge.app pid \(getpid())"]
        if let identity = PeerIdentity.signingIdentity(pid: getpid()) {
            lines.append(identity.description)
        } else {
            lines.append("unsigned: no code-signing identity (TCC evidence blocked)")
        }
        lines.append("LaunchAgent: \(launchAgent.statusDescription)")
        lines.append("plist present: \(launchAgent.plistIsPresent), daemon binary present: \(launchAgent.daemonBinaryIsPresent)")
        statusItem.button?.toolTip = lines.joined(separator: "\n")
    }

    // MARK: - Actions

    @objc private func handshakeAsApp() {
        runAsync(title: "Host IPC handshake (role app)") {
            DaemonClient.handshake(role: HostIPCRole.app)
        }
    }

    @objc private func handshakeAsDiagnostics() {
        runAsync(title: "Host IPC handshake (role diagnostics)") {
            DaemonClient.handshake(role: HostIPCRole.diagnostics)
        }
    }

    @objc private func registerAgent() {
        var lines: [String] = []
        lines.append("bundled plist: \(launchAgent.bundledPlistPath) (present: \(launchAgent.plistIsPresent))")
        lines.append("bundled daemon: \(launchAgent.bundledDaemonPath) (present: \(launchAgent.daemonBinaryIsPresent))")
        guard launchAgent.plistIsPresent else {
            present(title: "LaunchAgent register", body: lines.joined(separator: "\n") + "\n\nrefused: bundled plist missing")
            return
        }
        do {
            try launchAgent.register()
            lines.append("SMAppService.register() returned without error")
        } catch {
            lines.append("SMAppService.register() failed: \(error)")
        }
        lines.append("status after register: \(launchAgent.statusDescription)")
        let info = launchAgent.daemonJobInfo()
        lines.append("launchctl available: \(info.available), running: \(info.running), pid: \(info.pid.map { String($0) } ?? "none"), ppid: \(info.parentPID.map { String($0) } ?? "none")")
        lines.append(info.detail)
        present(title: "LaunchAgent register", body: lines.joined(separator: "\n"))
    }

    @objc private func unregisterAgent() {
        var lines: [String] = []
        do {
            try launchAgent.unregister()
            lines.append("SMAppService.unregister() returned without error")
        } catch {
            lines.append("SMAppService.unregister() failed: \(error)")
        }
        lines.append("status after unregister: \(launchAgent.statusDescription)")
        present(title: "LaunchAgent unregister", body: lines.joined(separator: "\n"))
    }

    @objc private func showDaemonStatus() {
        let info = launchAgent.daemonJobInfo()
        let body = [
            "LaunchAgent status: \(info.status)",
            "launchctl job available: \(info.available)",
            "running: \(info.running)",
            "pid: \(info.pid.map { String($0) } ?? "none")",
            "ppid: \(info.parentPID.map { String($0) } ?? "none")",
            "state: \(info.state ?? "unknown")",
            "program: \(info.program ?? "unknown")",
            info.detail,
        ].joined(separator: "\n")
        present(title: "Daemon (launchd view)", body: body)
    }

    @objc private func installExternalDaemon() {
        runAsync(title: "Install external daemon") {
            do {
                let installed = try self.installer.install()
                let snapshot = try DaemonLaunchctl.inspect(label: HostIPCPaths.daemonLaunchAgentLabel)
                return DaemonClient.Outcome(ok: true, body: [
                    "installed at: \(installed.url.path)",
                    "team: \(installed.teamIdentifier ?? "unknown")",
                    "cdhash: \(installed.cdhash ?? "unknown")",
                    "launchctl job: pid \(snapshot.pid.map { String($0) } ?? "none"), state \(snapshot.state ?? "unknown")",
                    "program: \(snapshot.program ?? "unknown")",
                ].joined(separator: "\n"))
            } catch {
                return DaemonClient.Outcome(ok: false, body: "install failed: \(error)")
            }
        }
    }

    @objc private func uninstallExternalDaemon() {
        runAsync(title: "Uninstall external daemon") {
            do {
                try self.installer.uninstall()
                return DaemonClient.Outcome(ok: true, body: "uninstalled (runtime data preserved)")
            } catch {
                return DaemonClient.Outcome(ok: false, body: "uninstall failed: \(error)")
            }
        }
    }

    @objc private func probeAsAppChild() {
        runAsync(title: "Probe permissions (CodeBridge.app child)") {
            ProbeChildRunner.run(probe: "permissions")
        }
    }

    @objc private func probeViaDaemon() {
        runAsync(title: "Probe permissions (codebridged child)") {
            DaemonClient.daemonAttributedProbe("permissions")
        }
    }

    @objc private func probeInputMonitor() {
        runAsync(title: "Probe input monitor (CodeBridge.app child)") {
            ProbeChildRunner.run(probe: "input-monitor")
        }
    }

    @objc private func showSigningIdentity() {
        var lines: [String] = []
        if let identity = PeerIdentity.signingIdentity(pid: getpid()) {
            lines.append(identity.description)
            lines.append("executable: \(ProcessIdentityReader.executablePath(pid: getpid()) ?? "unknown")")
        } else {
            lines.append("no code-signing identity available for this process")
        }
        let launch = ProcessIdentityReader.current()
        lines.append("launch: \(launch)")
        present(title: "Signing identity", body: lines.joined(separator: "\n"))
    }

    @objc private func showBundlePaths() {
        let body = [
            "bundle: \(Bundle.main.bundlePath)",
            "executable: \(ProcessIdentityReader.executablePath(pid: getpid()) ?? "unknown")",
            "plist: \(launchAgent.bundledPlistPath)",
            "daemon: \(launchAgent.bundledDaemonPath)",
            "probe: \(ProbeChildRunner.bundledProbePath() ?? "not bundled")",
            "host IPC socket: \(HostIPCPaths.socketPath)",
            "socket exists: \(FileManager.default.fileExists(atPath: HostIPCPaths.socketPath))",
        ].joined(separator: "\n")
        present(title: "Bundle paths", body: body)
    }

    @objc private func quit() {
        NSApp.terminate(nil)
    }

    // MARK: - Helpers

    private func runAsync(
        title: String,
        operation: @escaping () -> DaemonClient.Outcome
    ) {
        worker.async { [weak self] in
            let outcome = operation()
            DispatchQueue.main.async {
                self?.present(title: title, body: outcome.body)
            }
        }
    }

    private func present(title: String, body: String) {
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = ""
        alert.alertStyle = .informational
        alert.addButton(withTitle: "OK")

        let textView = NSTextView(frame: NSRect(x: 0, y: 0, width: 640, height: 380))
        textView.string = body
        textView.isEditable = false
        textView.isSelectable = true
        textView.font = NSFont.monospacedSystemFont(ofSize: 11, weight: .regular)

        let scrollView = NSScrollView(frame: NSRect(x: 0, y: 0, width: 640, height: 380))
        scrollView.documentView = textView
        scrollView.hasVerticalScroller = true
        scrollView.borderType = .bezelBorder
        alert.accessoryView = scrollView

        alert.runModal()
    }
}
