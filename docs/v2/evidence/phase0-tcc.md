# Phase 0 — TCC attribution, permission probes and protected roots (native side)

Status: **ARCHITECTURE_BLOCKER F2 — actual daemon-attributed capture succeeded; genuine signed isolation remains unverified. Phase 1 NOT READY.**

Owner: NativeSpike (`desktop/macos`, native evidence)
Frozen contract: [Architecture](../architecture.md) §5.1–§5.2, §11.8, §13.5, §13.6; [Provider Contracts](../provider-contracts.md) §6–§7; [Roadmap](../roadmap.md) Phase 0 "TCC attribution", "Daemon protected-root TCC", "Native host".

This document covers Roadmap tasks **5 (TCC attribution)**, **6 (daemon protected-root TCC)** and the native side
of **7 (native host)**. The signed-app half of each is explicitly left BLOCKED rather than guessed.

## 1. What was built

| Path | Purpose |
| --- | --- |
| `desktop/macos/Sources/codebridge-probe/` | standalone harness: `permissions`, `signing`, `native-host`, `harness`, `lock-state`, `input-monitor`, `all`, `list`, plus `--handshake-role <role>` for the Host IPC wire smoke |
| `desktop/macos/Sources/codebridge-probe/ScreenCaptureProbe.swift` | `CGPreflightScreenCaptureAccess()` + real `SCShareableContent.excludingDesktopWindows` call + one real `SCScreenshotManager.captureImage`; reports dimensions and a SHA-256 of the in-memory buffer, **never** writes pixels |
| `desktop/macos/Sources/codebridge-probe/AccessibilityProbe.swift` | `AXIsProcessTrusted()` + real `AXUIElementCopyAttributeValue(systemWide, "AXFocusedApplication")` |
| `desktop/macos/Sources/codebridge-probe/InputMonitorProbe.swift` | listen-only `.cgSessionEventTap`, conservative origin classification (no keystroke content) |
| `desktop/macos/Sources/codebridge-probe/FilesFoldersProbe.swift` | read-only `opendir` + entry **count** on protected roots, bounded per root |
| `desktop/macos/Sources/CodeBridgeApp/ProbeChildRunner.swift` | runs the harness as an app child; `CodeBridge --phase0-probe <probe>` makes that scriptable |
| `desktop/macos/Sources/CodeBridgeIPC/PeerIdentity.swift` | live code-signature inspection/verification (`SecCodeCopyGuestWithAttributes`, audit token via `LOCAL_PEERTOKEN`, `SecCodeCopySigningInformation`) |
| `desktop/macos/Sources/CodeBridgeIPC/PeerTrust.swift` | peer trust policy + Apple-anchored `SecRequirement` (`anchor apple generic …`), used for authorization via `SecCodeCheckValidity` on the live peer |

**Scope statement:** the Phase 0 native slice is a **Host IPC client only**. It registers no handler for the
daemon → app services (`computer.describe/targets/open_session/observe/act/control/close_session`,
`approval.present`, `approval.cancel`) and it does not serve `notify.post`. Those are Phase 1 work; nothing in this
document implies the ComputerProvider or approval presenter exists.

Safety invariants enforced in code and used for every measurement below: **no** `CGRequest*Access`, **no** AX
prompt option, **no** input injection, **no** pixel persistence, **no** file names from protected roots, **no**
`tccutil`, **no** lock/sleep trigger.

## 2. Observed — launch-context table (same ad-hoc binary, four contexts)

Host: macOS 27.0 (26A428), arm64, 0 code-signing identities, `gui/501` launchd domain.
Probe: `codebridge-probe permissions --json`, `Signature=adhoc`, `TeamIdentifier=not set`,
`signing_identifier=codebridge-probe` (`codebridge_app_child` run: bundle `com.codebridge.app`).

| Measurement | A. shell child (`ppid` = bun) | B. `launchctl submit` job (`ppid` = 1) | C. app child via LaunchServices | D. child of a development-mode `codebridged` |
| --- | --- | --- | --- | --- |
| `CGPreflightScreenCaptureAccess()` | false | false | false | false |
| real `SCShareableContent` call | denied, `SCStreamErrorDomain -3801` (TCC denied) | denied, `SCStreamErrorDomain -3801` | denied, `SCStreamErrorDomain -3801` | denied, `SCStreamErrorDomain -3801` |
| stop-capture call | skipped (no grant) | skipped | skipped | skipped |
| `AXIsProcessTrusted()` | **true** | **false** | **true** | **true** |
| real AX call (`AXFocusedApplication`) | error, `AXError -25204` (`kAXErrorCannotComplete`) | error, `-25204` | error, `-25204` | error, `-25204` |
| `CGPreflightListenEventAccess()` / `CGPreflightPostEventAccess()` | true / true | **false / false** | true / true | true / true |
| `IsSecureEventInputEnabled()` | false | false | true (varies) | false |
| listen-only `.cgSessionEventTap` | created | created | created | created |
| `~/Desktop`, `~/Documents`, `~/Downloads` `opendir` | readable, 3 / 7 / 230 entries | **blocked** (see §4) | readable, 3 / 7 / 230 | readable, 3 / 7 / 230 |

Context D was produced with the frozen `host.phase0_probe` path (DaemonBaseline's daemon, started in development
mode from a shell as [Architecture](../architecture.md) §5.3 allows):

```bash
CODEBRIDGE_PHASE0_DEBUG=1 CODEBRIDGE_PHASE0_PROBE=<abs>/codebridge-probe \
  bin/codebridged run -run-dir /tmp/cb-native-phase0/run -data-dir /tmp/cb-native-phase0/data -tunnel=false
bin/codebridged ctl probe --host-socket /tmp/cb-native-phase0/run/hostipc.sock --probe permissions
# probe reported: launch_context=codebridged_child, ppid=<daemon pid>, exit_code=0
```

### 2.0 Host IPC wire and peer authorization (observed end-to-end)

Against that same running daemon, the Swift client completed the frozen handshake:

```text
role=diagnostics  connected=true  negotiated_protocol=1.0  daemon_version=dev
peer credentials: pid=<daemon pid> uid=501 gid=20 auditToken=present; uid matched
peer signature read live from the audit token: identifier=a.out team=none adhoc=true valid=true
```

Peer-trust controls (all four are the frozen policy, implemented in `Sources/CodeBridgeIPC/PeerTrust.swift`):

| Control | Observed result |
| --- | --- |
| `role=app` **with** `CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER=1` | refused: "role=app requires CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID … the environment opt-in does not apply to the app role" — **no bypass** |
| `role=app` with `CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID=a.out`, `CODEBRIDGE_EXPECT_TEAM_ID=FAKETEAM123` against the ad-hoc daemon | refused by the **live** check: `SecCodeCheckValidity` failed with OSStatus **-67050** against `anchor apple generic and identifier "a.out" and certificate leaf[subject.OU] = "FAKETEAM123"` |
| `role=diagnostics` with the opt-in | connected (control) |
| `role=diagnostics` without the opt-in | refused (fail closed) |

Authorization therefore uses the live `SecCode` from the peer's audit token with an Apple-anchored
`SecRequirement`; static `SecCodeCopySigningInformation` output is used for reporting only, and ad-hoc peers can
never satisfy an Apple-anchored requirement. `mcp.sock` correctly did not answer `host.hello` (it is the MCP
ingress, not the Host IPC endpoint).

Commands (all repeatable, none request a permission):

```bash
P=desktop/macos/.build/release/codebridge-probe
$P permissions --json --pretty --output /tmp/shell.json
launchctl submit -l com.codebridge.phase0.p1 -- "$P" permissions --json --output /tmp/launchd.json
open -a desktop/macos/build/CodeBridge.app --args --phase0-probe permissions   # raw JSON in ~/Library/Logs/CodeBridge/
```

### 2.1 What this establishes (observed)

1. The **same binary** reports different Accessibility / Input Monitoring answers in different launch
   contexts (A/C true, B false). Permission state is therefore **not** a property of the probe binary alone on
   this host; the launch context decides. `[INFERENCE]` the mechanism is responsible-process attribution
   (A/C remain attached to the launching UI chain, B was submitted to launchd), but this is **not proven** by
   the observations above.
2. Screen Recording is false/denied in **all** contexts: no Screen Recording grant exists anywhere in these
   process chains, and the reserved-behaviour gap (child inheriting the app's grant) is **not** demonstrated.
3. `AXIsProcessTrusted() == true` does **not** imply the AX API works: the real `AXFocusedApplication` query
   failed with `kAXErrorCannotComplete` (‑25204) in every context, including contexts that reported
   `trusted = true`. Phase 1 must therefore verify a real AX call, not the trust boolean.
4. Context D shows the inheritance effect reaching **through the daemon to its children**: a probe launched by
   the frozen `host.phase0_probe` path (daemon in development mode, started from a shell) reported the same
   Accessibility / Input Monitoring answers as the shell child (A). `[INFERENCE]` a daemon's TCC posture is
   inherited from however the daemon itself was launched, so the production case (daemon owned by launchd,
   signed with its own identity) is the one that decides invariant 9 / frozen decision F2 — and it is **not**
   measured by any row above.

### 2.2 What this does NOT establish (BLOCKED)

| Claim | Status | Exact prerequisite |
| --- | --- | --- |
| A CodeBridge.app child inherits / does not inherit the app's Screen Recording, Accessibility or Input Monitoring grants | **BLOCKED** | genuine signing identity + Team ID, signed bundle, real TCC grants, then compare `permissions` from the app child against the daemon child |
| A `codebridged`-owned harness is **not** attributed to CodeBridge.app (invariant 9 / frozen decision F2) | **BLOCKED** | signed app **and** signed daemon, daemon running with `CODEBRIDGE_PHASE0_DEBUG=1`, `host.phase0_probe` (daemon child) vs `--phase0-probe` (app child) compared on the same host |
| The daemon's own Screen Recording / Accessibility / Input Monitoring posture | **BLOCKED** | measured only for a development-mode daemon (context D, started from a shell); the launchd-owned signed daemon is the case that matters and requires the signing prerequisite below |
| The app's role `app` Host IPC session | **refused by design** (observed) | role `app` requires a configured daemon signing identifier + non-empty Team ID and a live Apple-anchored signature check; with 0 identities every `app` connection is refused, which is the intended fail-closed behaviour, not a workaround target |
| Computer / approval / notify provider surface (daemon → app services) | **not implemented** | the Phase 0 native slice is **client-only**: it registers no handler for `computer.*`, `approval.present`/`cancel` or `notify.post`; those arrive in Phase 1 |
| Expected Team ID / peer signature verification for role `app` | **not configured** | `CODEBRIDGE_EXPECT_TEAM_ID`, `CODEBRIDGE_EXPECT_APP_SIGNING_ID`, `CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID` are unset; probe reports `team_identifier_configured=false` |

## 3. Bundle signing identity tree (observed)

```text
$ security find-identity -v -p codesigning
     0 valid identities found

$ codesign -dv --verbose=2 desktop/macos/build/CodeBridge.app
Identifier=CodeBridge
CodeDirectory v=20400 size=10115 flags=0x20002(adhoc,linker-signed) hashes=313+0 location=embedded
Signature=adhoc
TeamIdentifier=not set
Info.plist=not bound
Sealed Resources=none
Internal requirements=none

$ codesign -dv --verbose=2 <bundle>/Contents/MacOS/codebridge-probe
Identifier=codebridge-probe    Signature=adhoc    TeamIdentifier=not set

$ codesign -dv --verbose=2 <bundle>/Contents/MacOS/codebridged
Identifier=a.out               Signature=adhoc    TeamIdentifier=not set
```

Notes:
- the ad-hoc signatures are the toolchain's automatic `linker-signed` signature, **not** a genuine identity, and
  this document does not treat them as TCC-capable;
- `build-app.sh` refuses to run without `--sign-identity` + `--team-id` (only `--unsigned-dev` bypasses, and it
  then stamps `CodeBridgeUnsignedDevBuild=true` in `Info.plist` and writes `UNSIGNED-DEV-BUILD.txt`);
- the signed path signs nested binaries first and uses `--identifier com.codebridge.daemon` for `codebridged`,
  which is what the daemon's `SecRequirementCreateWithString(identifier …)` check needs;
- `Info.plist` also carries `NSDesktopFolderUsageDescription` / `NSDocumentsFolderUsageDescription` /
  `NSDownloadsFolderUsageDescription` and `LSUIElement=true`.

## 4. Protected roots (Roadmap task 6) — observed behaviour

`codebridge-probe permissions` performs a read-only `opendir` + entry count per root, **bounded at 5 s per root**:

| Context | `~/Desktop` | `~/Documents` | `~/Downloads` |
| --- | --- | --- | --- |
| A. shell child | readable, entries 3 | readable, entries 7 | readable, entries 230 |
| C. app child (LaunchServices) | readable, entries 3 | readable, entries 7 | readable, entries 230 |
| B. `launchctl submit` job | `readable=false, blocked_seconds=5.14` | `blocked_seconds=5.15` | `blocked_seconds=5.21` |

The first, unbounded run of context B was sampled while still blocked: `__opendir2` → `__open_nocancel`, i.e.
the process was blocked inside `open(2)` on a protected root for **83 seconds** and had to be killed. Later runs
were bounded by the probe's own deadline:

```text
$ sample <pid>    # during the unbounded run
+  __opendir2 (in libsystem_c.dylib) + 44
+     __open_nocancel (in libsystem_kernel.dylib) + 8
```

Interpretation (observed, not inferred): an unattributed, non-UI launchd-owned process did **not** receive a
TCC denial (`EPERM`) for these roots within the observation window; it blocked pending a decision that this
context apparently cannot present. This is the exact failure mode that [Architecture](../architecture.md) §13.5
requires Project registration to avoid (`permission_denied: host_permission_required` instead of an opaque I/O
failure) — and it additionally requires the daemon-side probe to be **time-bounded**, because a plain
`stat`/`open` can hang rather than fail.

### 4.1 Outcome decision — explicitly BLOCKED, not "unsupported"

Roadmap task 6 asks for one documented v1 outcome: supported-with-guidance, or explicitly unsupported. The
deciding evidence is *authorization persistence across rebuild/update for a genuine signed daemon*, which
**cannot be measured here**: this host has 0 valid signing identities, so there is no stable code-signing
identity to hold a Files-and-Folders grant across rebuilds. Per the architecture this is a **missing
prerequisite**, so the outcome stays `BLOCKED (untested)` and must **not** be recorded as `UNSUPPORTED`.

Prerequisite to close it (all on a machine with a Developer ID / Apple Development identity):

1. `build-app.sh --sign-identity "Developer ID Application: … (TEAMID)" --team-id TEAMID --daemon <signed codebridged>`;
2. register the LaunchAgent, start the daemon, project-register `~/Documents` (expect a Files-and-Folders
   decision attributed to the daemon, not to CodeBridge.app);
3. rebuild/re-sign with the **same** identity and team, restart the daemon, re-register the root, and observe
   whether the authorization persists;
4. record the outcome; if it does not persist, `protected roots unsupported in v1` may then be chosen **with
   evidence**.

Additionally: whatever the outcome, daemon-side root probing must be deadline-bounded (§4) and map
missing access to `permission_denied` reason `host_permission_required`.

## 5. Native host tool availability (Roadmap task 7, native side)

`codebridge-probe native-host --json` runs fixed, read-only argv checks and reports the launch context:

| Tool | Resolved path | Version (observed) |
| --- | --- | --- |
| shell | `/bin/zsh` | zsh 5.9 (arm64-apple-darwin26.0) |
| git | `/usr/bin/git` | git version 2.50.1 (Apple Git-155) |
| docker | `/usr/local/bin/docker` | Docker version 29.7.2, build a7dcaa6 |
| ssh | `/usr/bin/ssh` | OpenSSH_10.3p1, LibreSSL 3.3.6 |
| kubectl | `/usr/local/bin/kubectl` | Client Version: v1.36.1 |

Launch context of this measurement: `process_child:bun` (the developing shell), not `codebridged`. The
authoritative "works from `codebridged`" measurement is the daemon's own `host.native_host_smoke`
(DaemonBaseline slice); see `phase0-native-host.md` for that side and `phase0-ipc.md` for the Host IPC wire.
The daemon-child run of the *probe binary* measured from this side is context D in §2 above
(`launch_context = codebridged_child`, `ppid` = daemon pid).

## 6. Commands for the parent

```bash
# build + tests of the native slice (self-contained; does not touch Go)
cd desktop/macos && swift build -c release && swift test

# probes (each writes one JSON report; no permission is ever requested)
P=desktop/macos/.build/release/codebridge-probe
$P list    --json --pretty --output /tmp/cb-list.json
$P all     --json --pretty --output /tmp/cb-all.json          # permissions + signing + native-host
$P permissions --json --pretty --output /tmp/cb-permissions.json
$P signing --json --pretty --output /tmp/cb-signing.json
$P native-host --json --pretty --output /tmp/cb-native-host.json

# launch-context comparison (attribution proxy)
launchctl submit -l com.codebridge.phase0.p1 -- "$P" permissions --json --output /tmp/cb-launchd.json
launchctl remove com.codebridge.phase0.p1

# app-attributed run (needs an assembled bundle; the app is launched by LaunchServices so it is the
# responsible process, then it runs the probe as its own child). Raw JSON lands in ~/Library/Logs/CodeBridge/
open -a "$PWD/desktop/macos/build/CodeBridge.app" --args --phase0-probe permissions
ls -t ~/Library/Logs/CodeBridge/ | head -3

# daemon-attributed run: the daemon must be started with the Phase 0 gate, then it spawns the probe as ITS child
CODEBRIDGE_PHASE0_DEBUG=1 CODEBRIDGE_PHASE0_PROBE="$PWD/<abs>/codebridge-probe" \
  bin/codebridged run -run-dir /tmp/cb-native/run -data-dir /tmp/cb-native/data -tunnel=false &
bin/codebridged ctl probe --host-socket /tmp/cb-native/run/hostipc.sock --probe permissions

# peer-trust controls (all four verified; the first two must be refusals)
SOCK=/tmp/cb-native/run/hostipc.sock
CODEBRIDGE_HOSTIPC_SOCKET=$SOCK CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER=1 \
  $P --handshake-role app --json                                  # refuse: role=app has no bypass
CODEBRIDGE_HOSTIPC_SOCKET=$SOCK CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER=1 \
  CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID=a.out CODEBRIDGE_EXPECT_TEAM_ID=FAKETEAM123 \
  $P --handshake-role app --json                                  # refuse: live Apple-anchored check fails (-67050)
CODEBRIDGE_HOSTIPC_SOCKET=$SOCK CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER=1 \
  $P --handshake-role diagnostics --json                          # connect (control)
CODEBRIDGE_HOSTIPC_SOCKET=$SOCK $P --handshake-role diagnostics --json   # refuse: no opt-in
```

Note for tree scans: `desktop/macos/.build/` is SwiftPM's checkout/cache directory (ignored by
`desktop/macos/.gitignore`); exclude it and treat `Sources/`, `Tests/` and `Building/` as the native slice.

## 7. Parent integration: F2 stop condition

Environment: macOS 27.0 arm64, UID 501; manually bootstrapped per-user LaunchAgent, ad-hoc daemon/harness, no Team ID or genuine signing identity. This is not the signed `SMAppService` path.

Observed: daemon PID 62447 initially returned ScreenCapture `-3801`; Desktop/Documents/Downloads each hit the 5-second probe deadline. After an owned crash/restart (daemon PID 72321), its harness PID 77878 completed real capture: **1800 × 1169**, SHA-256 `5186ae37cda0a8d32859257201bce4fcff17c1ad106f001053adb7b55208f096`. Pixels were not persisted. Protected roots and ordinary `/tmp` were readable (counts 3/7/230/377). AX remained untrusted (real call `-25204`); Listen/Post preflights were false.

Read-only existing unified logs identify ScreenCapture request `77878.1`: responsible `a.out`, PID **72321**, path `bin/codebridged`; authorization subject is the same daemon path; `authValue=2`, `authReason=4`, no error. Exact command and selected original records: [attribution log](phase0-tcc-attribution.txt). Full sanitized measurements: [parent smoke](phase0-parent-smoke.json).

Conclusion: **F2 cannot pass**. A real daemon-owned harness captured with daemon-attributed authorization. This does **not** establish causal inheritance of a genuinely signed App grant: the original grant source is unknown, and the environment had no genuine signed identities. Earlier launch-context denials are time-specific measurements, not a durable safety guarantee. Task 6 cannot honestly choose `SUPPORTED` or `UNSUPPORTED_IN_V1` without the required signed rebuild/update comparison.

Architecture impact: `ARCHITECTURE_BLOCKER F2`; Computer permission/input experiments stopped, owned LaunchAgent booted out. Frozen topology remains unchanged. Minimum next action is the **already frozen F2 contingency**: independently signed daemon identity and disclaimed-responsibility harness spawning, then a controlled genuine App/daemon/harness grant comparison. No architecture redesign or automatic TCC modification was performed.

