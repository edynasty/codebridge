# Phase 0 — Lifecycle spike: `SMAppService` LaunchAgent, `KeepAlive`, app-quit survival

Status: **IMPLEMENTATION OBSERVED / signed-path outcomes BLOCKED (missing prerequisite: signing identity + Team ID)**

Owner: NativeSpike (`desktop/macos`, native evidence)
Frozen contract: [Architecture](../architecture.md) §5.1–§5.3, §5.5; [Provider Contracts](../provider-contracts.md) §7; [Roadmap](../roadmap.md) Phase 0 "Lifecycle".

## 1. What was built

| Path | Purpose |
| --- | --- |
| `desktop/macos/Sources/CodeBridgeApp/main.swift`, `AppDelegate.swift` | menu-bar app (`LSUIElement`, accessory activation policy), no Dock icon |
| `desktop/macos/Sources/CodeBridgeApp/MenuBarController.swift` | menu-bar surface: register/unregister LaunchAgent, `launchctl` view, Host IPC handshake, probe runs |
| `desktop/macos/Sources/CodeBridgeApp/LaunchAgentController.swift` | `SMAppService.agent(plistName:)` register/unregister/status; reads `launchctl print gui/<uid>/com.codebridge.daemon` and the daemon's parent pid |
| `desktop/macos/Sources/CodeBridgeApp/DaemonClient.swift`, `ProbeChildRunner.swift` | Host IPC handshake from the app; app-child probe run |
| `desktop/macos/Sources/CodeBridgeIPC/PeerTrust.swift` | peer trust policy: `role=app` never bypasses verification and requires identifier + non-empty Team ID |
| `desktop/macos/Building/com.codebridge.daemon.plist.in` | LaunchAgent plist template (`Label`, `BundleProgram`, `KeepAlive.SuccessfulExit=false`, `RunAtLoad`, `ThrottleInterval`; optional `EnvironmentVariables`) |
| `desktop/macos/Building/build-app.sh` | assembles `CodeBridge.app`; requires a genuine signing identity unless `--unsigned-dev` is passed |

Lifecycle ownership implemented as frozen: launchd owns `codebridged`. `LaunchAgentController` only calls
`SMAppService.register()/unregister()` and `launchctl print` (read-only inspection). **The app never spawns the
durable daemon as a child**; the only child process the app starts is the transient `codebridge-probe` permission
harness (`ProbeChildRunner`). `BundleProgram = Contents/MacOS/codebridged` is an app-bundle relative path, as
required by `launchd.plist(5)` for `SMAppService` plists.

The Phase 0 app is a Host IPC **client only**: it serves none of the daemon → app services (`computer.*`,
`approval.present`, `approval.cancel`, `notify.post`). Its `role=app` connections additionally require a configured
daemon signing identifier plus a non-empty Team ID and a live Apple-anchored signature check
(`Sources/CodeBridgeIPC/PeerTrust.swift`); with 0 identities every app-role connection is refused locally, which is
the intended fail-closed behaviour while the signing prerequisite is missing.

## 2. Observed on this host (2026-10-02)

Host: macOS 27.0 (build 26A428), arm64, Xcode 26.4 (17E192), Swift 6.3, Go 1.26.5. Per-user GUI domain
`gui/501` exists. `security find-identity -v -p codesigning` → **0 valid identities found**
(also asserted by `build-app.sh`, which prints the same result).

```text
$ desktop/macos/Building/build-app.sh --unsigned-dev --daemon ../../bin/codebridged --embed-phase0-env
== observed code-signing identities ==
     0 valid identities found
...
plutil -lint .../Info.plist                                  -> OK
plutil -lint .../Library/LaunchAgents/com.codebridge.daemon.plist -> OK
app bundle:  desktop/macos/build/CodeBridge.app
signing mode: unsigned-dev
team id: <none>
daemon embedded: true
```

Bundled LaunchAgent plist as assembled (`plutil -p`):

```json
{
  "BundleProgram" => "Contents/MacOS/codebridged",
  "EnvironmentVariables" => {
    "CODEBRIDGE_DAEMON_VERSION" => "phase0",
    "CODEBRIDGE_HOSTIPC_SOCKET" => "<HOME>/Library/Application Support/CodeBridge/run/hostipc.sock",
    "CODEBRIDGE_PHASE0_DEBUG" => "1",
    "CODEBRIDGE_PHASE0_PROBE" => "<bundle>/Contents/MacOS/codebridge-probe"
  },
  "KeepAlive" => { "SuccessfulExit" => false },
  "Label" => "com.codebridge.daemon",
  "ProcessType" => "Interactive",
  "RunAtLoad" => true,
  "ThrottleInterval" => 10
}
```

Bundle layout observed: `Contents/MacOS/{CodeBridge, codebridge-probe, codebridged}`,
`Contents/Library/LaunchAgents/com.codebridge.daemon.plist`, `Contents/Info.plist`, and (unsigned-dev only)
`Contents/Resources/UNSIGNED-DEV-BUILD.txt`.

App launch smoke (menu-bar app previously closed with `SIGTERM`):

```text
$ <bundle>/Contents/MacOS/CodeBridge &
app alive: pid=<n> state=S  comm=<bundle>/Contents/MacOS/CodeBridge
terminated on SIGTERM
```

Host IPC state at measurement time: `~/Library/Application Support/CodeBridge/runtime.db` exists (0600, owned by
the Store slice) but there is **no `run/` directory and no `hostipc.sock`**, so no `codebridged` was listening
during this measurement.

## 3. Untested — exact prerequisites

| Claim in the roadmap | Status | Prerequisite to measure it |
| --- | --- | --- |
| `SMAppService` registers the daemon as a per-user LaunchAgent | **BLOCKED** | genuine signing identity + Team ID; `build-app.sh --sign-identity "…(TEAMID)" --team-id TEAMID --daemon <signed codebridged>`; then menu item *LaunchAgent: register daemon* |
| Quitting the app leaves the daemon running | **BLOCKED** | requires the previous row; observed sequence: register → `launchctl print gui/501/com.codebridge.daemon` shows `pid` with `ppid 1` → quit app → re-check the same pid |
| A daemon crash restarts (`KeepAlive`) | **BLOCKED** | requires the previous row; `kill -9 <daemon pid>` then observe a new pid; `KeepAlive.SuccessfulExit=false` is in the plist |
| App is a login item; daemon is not an app child | **design observed** | the plist/`SMAppService` path is the only daemon start path in code; `LaunchAgentController` reports the daemon's real parent pid (`ProcessIdentityReader.parentPID`) so the "`ppid == 1`" claim can be checked when the signed path exists |

No claim is made in this document that registration succeeds. With 0 identities the bundle is marked
`CodeBridgeUnsignedDevBuild=true` in `Info.plist`, carries `Signature=adhoc`, `TeamIdentifier=not set`, and the
script refuses to call it signed.

## 4. Commands for the parent

```bash
# read-only identity/domain report (writes nothing, requests no permission)
desktop/macos/Building/check-signing-identity.sh

# unsigned development bundle (never valid for TCC/SMAppService evidence)
desktop/macos/Building/build-app.sh --unsigned-dev --daemon <path-to-codebridged> --embed-phase0-env

# signed bundle (genuine identity required; the script refuses otherwise)
desktop/macos/Building/build-app.sh \
  --sign-identity "Developer ID Application: <name> (TEAMID)" --team-id TEAMID \
  --daemon <path-to-codebridged> --embed-phase0-env

# after installing the signed bundle, verify from a shell:
launchctl print "gui/$(id -u)/com.codebridge.daemon" | grep -E "state|pid|program"
```

The menu bar exposes the same checks: *LaunchAgent: register daemon*, *LaunchAgent: unregister daemon*,
*Daemon: launchd status*.

## 5. Parent manual LaunchAgent smoke (not SMAppService proof)

An owned fixture was bootstrapped in `gui/501` with `RunAtLoad`, `KeepAlive.SuccessfulExit=false` and two-second throttle. Actual daemon PID **62447**, parent **1**, cwd `/`; the daemon served both permission-0600 UDS endpoints and a schema-v1 WAL store. Killing only that owned PID with SIGKILL produced daemon PID **72321**, parent **1**, launch count **2**, last exit signal **Killed: 9**. The store reopened; the per-launch bearer rotated (old token HTTP 401, new token HTTP 200). The exact fixture was then booted out.

[Sanitized parent evidence](phase0-parent-smoke.json) excludes raw inherited GUI environment and bearer values. Conclusion: manual per-user LaunchAgent independence, crash restart and store reopen PASS; genuine signed `SMAppService` registration and **App quit after that registration remain BLOCKED**, not replaced by this smoke. F2 capture isolation also blocks Phase 1.

