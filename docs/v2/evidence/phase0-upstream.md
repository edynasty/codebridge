# Phase 0 — upstream CodexBridge audit

## Assumption

`Fanch-hui/codex-bridge` is the identified upstream ([Migration](../migration.md) §4.1) and is consumed **only behind CodeBridge-owned adapters**; upstream never owns the CodeBridge MCP surface, session model or domain types. This task pins the exact upstream revision, records LICENSE/NOTICE obligations, audits the stack/module boundaries, fills the capability matrix, and builds the upstream **unchanged** to decide a reuse mode per module. It does not import upstream code in Phase 0.

Reuse modes are the frozen order ([Migration](../migration.md) §4.3): (1) package dependency at a pinned version, unmodified; (2) copy with attribution; (3) vendored subtree with a patch queue; (4) fork/run the upstream app — **rejected**. CodeBridge.app keeps its own bundle id and signing identity; TCC grants are never shared with an upstream app (§4.7).

## Environment

- 2026-10-02, macOS 27.0 (arm64), workstation `tangxingpeng`.
- Toolchain observed: `swift --version` → Apple Swift 6.3 (swiftlang-6.3.0.123.5), target `arm64-apple-macosx28.0`; `xcodebuild -version` → Xcode 26.4 (17E192); `xcrun --find swiftc` → Xcode Default toolchain.
- `security find-identity -v -p codesigning` → **0 valid identities**; `tunnel-client` not on `PATH`.
- External checkout (read/build only, nothing written back upstream): `/tmp/codex-bridge-upstream`.
- Network reachable (git clone + `gh api` + SwiftPM fetches all succeeded).

## Commands / steps

```bash
# 1. checkout exactly the released revision
git clone https://github.com/Fanch-hui/codex-bridge.git /tmp/codex-bridge-upstream
cd /tmp/codex-bridge-upstream
git fetch origin && git rev-parse origin/win   # 7844bb608a9a4e96ed09c084589b7825db77aa3e
git describe --tags --always                    # v1.3.4
git rev-list --left-right --count HEAD...origin/win   # 0  0 (no drift)

# 2. unchanged SwiftPM build
cd Packages/BridgeCore && swift build

# 3. unchanged daemon smoke (isolated temp data root, no TCC, no installer)
./.build/debug/codex-bridge-service --foreground --data-root /tmp/cb-smoke4

# 4. documented app build (README_en.md "Build from source")
Scripts/with-xcode.sh xcodebuild -project CodexBridge.xcodeproj -scheme CodexBridge \
  -configuration Debug -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath .build/Xcode build CODE_SIGNING_ALLOWED=NO

# 5. documented test command (CONTRIBUTING.md)
swift test --package-path Packages/BridgeCore

# 6. release metadata / license evidence
gh release download v1.3.4 -R Fanch-hui/codex-bridge -p SBOM.spdx.json -p SHA256SUMS -p latest.json
gh api repos/Fanch-hui/codex-bridge --jq .license.spdx_id
```

## Observed result

### 1. Pin (observed)

| Field | Observed value |
| --- | --- |
| Repository | `https://github.com/Fanch-hui/codex-bridge` |
| Default/development branch | `win` (`origin/HEAD → origin/win`) |
| **Exact commit** | `7844bb608a9a4e96ed09c084589b7825db77aa3e` |
| Tag | `v1.3.4` (lightweight tag, `git cat-file -t` → `commit`) |
| Commit date / author | 2026-10-01 15:44:22 -0400, `yeyuancc0-glitch` |
| GitHub Release | `Codex Bridge 1.3.4`, published 2026-10-02T03:46:02Z, not prerelease/draft |
| License (API + file) | Apache-2.0 |
| Repo provenance | 365 commits, 1 contributor, first commit 2026-08-12, 72 stars, 11 releases |
| `main` vs `win` | `main` is stale (`v0.4.0`, 2026-08-30); `win` is 365 commits ahead — **pin `win`** |

Other live branches: `codex/platform-native-20261002`, `codex/pi-qoder-integration-20260926`, `codex/review-20260927-fixes`, `ci/windows-codex-path-fix`. Active single-author development.

Pinned source links (all at `7844bb6…`):
- tree — https://github.com/Fanch-hui/codex-bridge/tree/7844bb608a9a4e96ed09c084589b7825db77aa3e
- LICENSE — https://github.com/Fanch-hui/codex-bridge/blob/7844bb608a9a4e96ed09c084589b7825db77aa3e/LICENSE
- NOTICE — https://github.com/Fanch-hui/codex-bridge/blob/7844bb608a9a4e96ed09c084589b7825db77aa3e/NOTICE
- Package manifest — https://github.com/Fanch-hui/codex-bridge/blob/7844bb608a9a4e96ed09c084589b7825db77aa3e/Packages/BridgeCore/Package.swift
- dependency doc — https://github.com/Fanch-hui/codex-bridge/blob/7844bb608a9a4e96ed09c084589b7825db77aa3e/docs/DEPENDENCIES.md
- tunnel contract — https://github.com/Fanch-hui/codex-bridge/blob/7844bb608a9a4e96ed09c084589b7825db77aa3e/docs/TUNNEL_CLIENT_INTEGRATION.md
- release process — https://github.com/Fanch-hui/codex-bridge/blob/7844bb608a9a4e96ed09c084589b7825db77aa3e/docs/RELEASE.md
- release — https://github.com/Fanch-hui/codex-bridge/releases/tag/v1.3.4

### 2. LICENSE / NOTICE and obligations (observed)

- `LICENSE` = verbatim Apache-2.0 (201 lines, sha256 `ae0d8300…5806f`), unmodified.
- `NOTICE` (958 B, sha256 `574e1aba…147a03`) carries the attribution upstream itself must preserve, and which we must propagate on any reuse:
  - `Codex Bridge — Copyright 2026 Codex Bridge contributors`;
  - `SwiftLog` (Copyright 2018, 2019 The SwiftLog Project), which derives from `SwiftNIO`;
  - `SwiftNIO` (Copyright 2017, 2018 The SwiftNIO Project), deriving from Netty, NodeJS llhttp, uSHET, FreeBSD, swift-base64-kit, AsyncHTTPClient;
  - Release builds may bundle `[OI]` `tunnel-client`; its pinned LICENSE/NOTICE/supply manifest are copied into the bundle under `Contents/Resources/TunnelClient` and verified before packaging.
- Obligations if any module is reused (mode 2 or 3): keep the Apache-2.0 `LICENSE`, keep and extend the `NOTICE`, state the origin repo + commit + files, mark modified files, and — because `BridgeSecurity`/`BridgeMCP`/`BridgeTunnel` link Apache-2.0 deps — carry the transitive notices listed below. CodeBridge.app must not share TCC grants or bundle id.
- Published release `SBOM.spdx.json` (SPDX-2.3, 11 packages) declares `Apache-2.0` for `CodexBridge` itself but **`NOASSERTION` for every third-party package** — the published SBOM does not itself satisfy third-party notice obligations. Do not rely on it as license evidence; the manifests below are the source of truth.
- `docs/DEPENDENCIES.md` claims the pinned helper SHA-256 values live in that file, but they are actually hardcoded in `Scripts/build-tunnel-helper.sh` (doc drift, cosmetic).

### 3. Stack and repository audit (observed)

- Language/stack: **Swift 6.1 tools / Swift 6.0 language mode, `StrictConcurrency: complete`**, macOS 14+ deployment, SwiftUI + AppKit + WebKit host, GRDB-backed SQLite, Swift NIO HTTP, Swift Crypto, vendored MCP Swift SDK, XPC (macOS) / named pipes (Windows). Windows shell is the same Swift package (`BridgeWindowsShell`, WebView2).
- Size: 1,103 tracked files, ~29 MB working tree; **915 Swift files, 146,782 Swift lines** (excl. `.build`); **8,393 lines** of HTML/CSS/JS desktop-UI resources. No vendored fork of an app in-tree.
- Two build systems on the same sources: SwiftPM package `Packages/BridgeCore` (`BridgeCore`) **and** an Xcode project `CodexBridge.xcodeproj` that references `Packages/BridgeCore` as a local Swift package and adds the `.app` + embedded service targets.
- Layout:

```text
App/            CodexBridgeApp.swift (SwiftUI @main, MenuBarExtra), entitlements (empty <dict/>), LaunchAgent plist, Assets
Service/        CodexBridgeServiceMain.swift (daemon entry → ServiceProcessRunner)
Packages/BridgeCore/
  Sources/      30 Swift targets (BridgeAgentCore, BridgeMCP, BridgeTunnel, BridgeSecurity, …)
  Windows/      app manifest + .rc
Vendor/swift-sdk/  vendored MCP Swift SDK 0.12.1 (guards EventSource import for Windows)
Integrations/MCPB/ optional MCP Bundle connector (Node)
Scripts/        33 build/sign/notarize/update/export scripts (zsh, python3, node, pwsh)
Config/         Base/Debug/Release/Signing.xcconfig(.example)
Windows/Installer/ Inno Setup scripts
docs/           24 design/connection/release docs
.github/workflows/ windows.yml, mcp-registry.yml
CodexBridge.xcodeproj/
```

- External Swift dependencies (`Packages/BridgeCore/Package.swift`, `Package.resolved`), with licenses read from each checkout's LICENSE:

| Dependency | Pin | License |
| --- | --- | --- |
| MCP Swift SDK | vendored `Vendor/swift-sdk` (upstream 0.12.1 + Windows shim) | MIT/Apache-2.0 transition + CC-BY-4.0 docs |
| GRDB.swift | 7.11.1 | MIT |
| swift-log | 1.15.0 | Apache-2.0 |
| swift-nio | **fork** `yeyuancc0-glitch/swift-nio@1a69138c…` (upstream NIO 2.101.3 + Windows loopback wakeup change) | Apache-2.0 + NOTICE chain (Netty/llhttp/uSHEt/FreeBSD/base64-kit/AHC) |
| swift-crypto | 3.12.0 | Apache-2.0 |
| eventsource / swift-system / swift-asn1 / swift-atomics / swift-collections | 1.5.1 / 1.8.0 / 1.7.1 / 1.3.1 / 1.6.0 | MIT / Apache-2.0 ×4 |

  Non-Swift runtime inputs: `[OI] tunnel-client` v0.0.10 (Apache-2.0, fetched+digest-verified at build time, not committed in-tree), MCPB Node bundle (`@modelcontextprotocol/sdk`, MIT/ISC/BSD per lockfile), Windows WebView2 SDK 1.0.4191.47 / vcpkg SQLite / Inno Setup 7.1.0. The vendored `Vendor/swift-sdk/Package.resolved` is **stale/misleading** (pins eventsource 1.1.0, apple NIO 2.94.0, docc-plugin branch main) and is not the effective resolution; the unified build uses the root `Package.resolved`. The published SPDX SBOM asserts `NOASSERTION` for every dependency license and omits `tunnel-client` entirely.
- **No macOS CI and no root SwiftPM manifest.** `.github/workflows/` has only `windows.yml` and `mcp-registry.yml` (macOS release is manual); `CONTRIBUTING.md` references a `ci.yml` that is absent. There is **no `Package.swift` at the repository root** — SwiftPM manifests exist only at `Packages/BridgeCore/Package.swift` and `Vendor/swift-sdk/Package.swift`, so upstream **cannot** be consumed as a remote SwiftPM package dependency; any SwiftPM reuse requires a local `path` dependency after vendoring (`[Observed]`, `git ls-tree HEAD` + `find -name Package.swift`).
- **No public test target**: zero `XCTest`/`swift-testing` files (`grep -rl "import XCTest\|import Testing\|@Test"` → none). `Scripts/export-public-source.py` explicitly exports a production-only tree and strips tests/fixtures; `docs/RELEASE.md` describes "automated tests" that are **not** in the public tree. So "run upstream tests" is impossible from this checkout; the build + smoke below are the only verifiable acceptance.
- MCP surface exposed to ChatGPT/Qwen (`BridgeMCP/MCPServiceToolCatalog.swift`, `contractVersion = "1.3.4"`): **42 tools** — `bridge_status`, `list_projects`, `list_agents`, `get_project`, `search_project_files`, `list_project_directory`, `batch_read_project_files`, `read_project_file`, `list_threads`, `read_thread`, `list/read/index/rename/delete_agent_native_session`, `list_models`, `list_agent_models`, `list_skills`, `read_skill`, `run_skill_action`, `list_tasks`, `get_task`, `wait_task`, `answer_user_input`, `submit_task`, `steer_task`, `interrupt_task`, `get_project_changes`, `list_project_commands`, and the `direct_*` family (write/edit/apply-patch/manage-path/preview-mutation/apply-mutation/undo-mutation/exec-command/read-command/write-stdin/interrupt-command/git-commit). Per [Architecture](../architecture.md) §16 and [Migration](../migration.md) §4.4 this catalogue is **not** reused: CodeBridge owns its MCP surface and upstream never registers or routes CodeBridge tools.

### 4. Module inventory (observed, LOC excl. resources)

| Module | Swift LOC | Role |
| --- | ---: | --- |
| `BridgeWindowsShell` | 14,271 | Windows SwiftUI/WebView2 shell |
| `BridgeMCP` | 11,807 | local MCP server (HTTP/loopback + stdio), tool catalog/dispatch |
| `BridgeServiceCore` | 10,212 | GRDB store, tasks, projects, agents, settings |
| `BridgeServiceApplication` | 9,156 | service application use-cases |
| `BridgeServiceHost` | 8,829 | daemon composition, IPC listener, agent discovery, tunnel wiring |
| `BridgeServiceAppShell` | 8,053 | macOS SwiftUI shell, service registration, updater |
| `BridgeCodexService` | 5,890 | Codex execution/supervision |
| `BridgeDeepSeekHarnessACP` | 5,687 | DeepSeek Harness ACP adapter |
| `BridgeServiceAppCore` | 5,204 | shared app core models/IPC client |
| `BridgeCodexRPC` | 5,141 | Codex app-server JSON-RPC client |
| `BridgeIPC` | 4,479 | XPC / named-pipe transports, codec, DTOs |
| `BridgeSecurity` | 4,174 | Keychain, identities, hashing, policy |
| `BridgeFiles`/`BridgeDirectCommand`/`BridgeGit` | 3,824 / 3,697 / 3,670 | file, direct-command and git engines |
| `BridgeAntigravityCLI`/`BridgeOpenCodeACP`/`BridgePiRPC`/`BridgeQoderSDK` | 3,469 / 3,276 / 3,217 / 2,126 | per-harness adapters |
| `BridgeDesktopUI` | 3,218 (+8.4k JS/CSS/HTML) | web UI resources |
| `BridgeTunnel` | 3,077 | `[OI]` tunnel-client supervision |
| `BridgeAgentCore` | 2,994 | agent provider protocol/descriptors |
| `BridgeDomain`/`BridgeProcess`/`BridgeSkills`/`BridgeLegacyImport`/`BridgeACP`/`BridgeProjects` | 1,602 / 1,353 / 1,336 / 1,286 / 748 / 562 | domain, process, skills, import, ACP base, projects |
| executables | 57 | `codex-bridge-service`, `codex-bridge-windows-app` |

**Negative finding (critical):** upstream contains **no Computer Use implementation**. `grep` for `CGEvent|ScreenCaptureKit|AXUIElement|SCShareableContent|CGDisplayStream|CGWindowList|CGImage|AVFoundation|NSAppleScript|RTCPeerConnection|WebRTC` across all `.swift`/`.js`/`.mjs` → **0 hits**; the only `accessibility…` matches are SwiftUI `accessibilityLabel` strings; `App/CodexBridge.entitlements` is an empty `<dict/>`. Upstream is an **agent orchestrator / host bridge with local approvals**, not a computer-use engine. CodeBridge's `computer.observe`/`computer.input`/`computer.preview` must be built by CodeBridge.

### 5. Capability matrix (observed source unless marked Untested)

Legend — Exists: yes/partial/no; Reusable: yes (usable behind an adapter as-is) / partial / no; Mode: reuse mode 1–4 or "not reused"; Patch risk: expected cost of adapting (low/medium/high).

| Capability | Exists | Location (pinned @7844bb6) | Dependencies | Reusable | Mode | Patch risk | Fallback |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Native Desktop Shell** | yes | `App/CodexBridgeApp.swift:1-53` (SwiftUI `@main`, `@NSApplicationDelegateAdaptor`, `WindowGroup`, `MenuBarExtra` tray, `.defaultSize(1180×760)`); `BridgeServiceAppShell/` thin host (root view 130 LOC, `BridgeDesktopWebView` 163, `ChatGPTWebView` 276); **the real UI is local HTML/CSS/JS** in `BridgeDesktopUI/Resources/` (~9.7k lines: `index.html`, `pages-*.js`), loaded into a `WKWebView` (`loadFileURL`, CSP `default-src 'self'`, non-persistent store) and driven by an `applyStatePatch`/`bridgeDesktopUI` message bridge | SwiftUI, AppKit, WebKit, ServiceManagement; BridgeIPC/BridgeServiceAppCore | partial — `BridgeDesktopUI` is host-neutral (depends only on BridgeServiceAppCore + BridgeAgentCore, also drives the Windows shell, `host-context.js` switches on `?platform=windows`); the SwiftUI host is thin but `BridgeServiceAppShell` is macOS-only and pulls IPC/MCP/AppCore and hardcodes mach service `org.codexbridge.service` | 2 (copy with attribution) for `App/*` + `BridgeDesktopUI`; not reused as a whole shell | high (whole-shell dependency) / low (App + xcconfig ~60 LOC) | own SwiftUI menu-bar app ([Migration](../migration.md) §4.2) |
| **Secure MCP Tunnel UX** | yes | `BridgeTunnel/` (11 files, ~2.3k LOC: `TunnelManager` 494, `TunnelProcessLauncher` 379, `TunnelHelperVerifier` 242, `UnixHealthClient` — actually `LoopbackHealthClient` — 328, `SecureRunDirectory`, `RedactedOutputBuffer`, `TunnelDoctorCompatibility`); `BridgeServiceHost/ServiceTunnelController.swift` (655), `BundledServiceTunnelManagerFactory`, `TunnelReconnectBackoff`; UI `pages-connections.js` / `pages-connections-editor.js`; `docs/TUNNEL_CLIENT_INTEGRATION.md` | `[OI]` `tunnel-client` v0.0.10 binary, Security.framework (`SecCode*`), Darwin (`posix_spawn`, `proc_pidinfo`, `openat`), swift-crypto, Keychain | yes — a self-contained hardening kit directly on CodeBridge's ingress path: fd-delivered secrets (fd 3 = Runtime Key, fd 4 = local MCP header secret), suspended `posix_spawn` + live-identity re-check, dirfd dev/inode run-dir binding, helper-PID-owns-port check, bounded redacted output, `doctor --json` preflight, 2-layer restart/backoff | 2 (copy with attribution) or 3 (vendored subtree) — **mode 1 is impossible: there is no `Package.swift` at the repo root**, so a remote SwiftPM dependency cannot resolve `BridgeTunnel` | medium (PINNED to the `tunnel-client` v0.0.10 CLI/`/readyz`/metric contract) | own setup screen; `codebridged` supervises `tunnel-client` |
| **Packaging** | yes | `CodexBridge.xcodeproj/project.pbxproj:149-192` (2 native targets: app + `CodexBridgeService` tool; phases `Embed Service`, `Copy LaunchAgent` → `Contents/Library/LaunchAgents`, `Stage Embedded Service Resources`, `Sign Embedded Service`, `Stage Tunnel Helper` at `:311-319`); `Scripts/build-release-candidate.sh` (archive → ad-hoc re-sign → ZIP + DMG + SBOM + `SHA256SUMS` + `latest.json`); `stage-embedded-service-resources.sh`, `generate-sbom.swift` | xcodebuild (complete Xcode), `ditto`, `hdiutil`, `lipo`, `codesign`, python3, `shasum` | partial — plain shell/python/swift scripts, coupled to upstream only by strings (`CodexBridge.app`, `Contents/Resources/CodexBridgeService`, `Contents/Helpers/tunnel-client`, `BridgeCore_*.bundle`) and repo identity; `arm64`-only gate | 2 (copy with attribution) | medium | own pipeline |
| **Signing** | yes | `Scripts/build-release-candidate.sh:125-136` (ad-hoc `codesign --force --sign -` for service/helper/dylibs then the app), `Scripts/stage-tunnel-helper.sh:107`, `Scripts/sign-embedded-service.sh:32-42` (Developer-ID branch, only when a real identity exists), `Scripts/verify-release-hardening.sh`; `Config/Signing.xcconfig.example` (**never `#include`d**) | Developer ID identity (0 on this host), `codesign`, Hardened Runtime flag | partial — **public releases are ad-hoc and Hardened Runtime is NOT applied** (ad-hoc calls pass no `--options runtime`; the archive uses `CODE_SIGNING_ALLOWED=NO`, so `ENABLE_HARDENED_RUNTIME=YES` is neutralised); the hardening verifier is a reusable gate | 2 | low–medium | own pipeline |
| **Notarization** | docs only / no | `docs/RELEASE.md:107-120` (notarytool/stapler/spctl sketch, "not used for v0.4.0"); `Scripts/build-release-candidate.sh:184` writes `Apple notarization: unavailable` into `RELEASE-INFO.txt`. **No notarytool/stapler/spctl anywhere in code or CI** | `xcrun notarytool` + release-owned Keychain profile (absent) | no code to reuse — procedure sketch only | not reused (lift the documented procedure into CodeBridge's own pipeline) | n/a | own pipeline |
| **Updater** | yes | `BridgeServiceAppCore/AppUpdate{Manifest,Client,Download,Controller}.swift` (self-contained: static JSON feed, exact `(platform,architecture,kind)` match, streaming SHA-256 + size verify, install gate, state machine); `BridgeServiceAppShell/MacAppUpdate{Helper,Installer,ServiceExit}.swift`; `BridgeDesktopUI/BridgeDesktopAppUpdateState.swift` + `Resources/app-update.js`; `Scripts/generate-update-manifest.py`; release `latest.json` | GitHub Releases `latest.json` (`https://github.com/Fanch-hui/codex-bridge/releases/latest/download/latest.json` hardcoded), `ditto`, `lipo`, `codesign`, `renameatx_np(RENAME_SWAP)`, `open`, `pgrep` | partial — the 4 feed/download/controller files are clean, but they live in `BridgeServiceAppCore` which depends on BridgeAgentCore + BridgeIPC + BridgeMCP, so **do not take it as a package dependency**; the macOS installer hardcodes `/Applications/CodexBridge.app`, the embedded service path and bundle id | 2 + patches | low (feed/client) / high (macOS installer) | own updater (e.g. Sparkle) or manual |
| **Keychain** | yes | `BridgeSecurity/KeychainSecretStore.swift` (146 LOC), `SecretStoreFactory.swift` (macOS Keychain / Windows Credential Manager); used for tunnel Runtime Key (`service.tunnel-runtime-key`) and DSH API key (`agent.deepseek-harness.api-key.<sha256(configPath)>`) | Security.framework (`SecItem*`); Windows Credential Manager branch | yes — self-contained `SecretStore` protocol (`store`/`load`/`remove`), generic-password items, `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`, 16 KiB cap, `init(service:)` is configurable (default `com.openai.codex-bridge.secrets`), **no Keychain access group / app group** | 2 (copy with attribution) | low | own Keychain wrapper |
| **Approval UI** | yes | `BridgeServiceApplication/DirectActionApprovalCenter.swift` (213 LOC actor: in-memory pending map, 300 s approval TTL / 30 s deny TTL, SHA-256 payload-digest binding, grant consumption); `BridgeServiceApplication+TaskApprovals.swift` (task-start approval **derived** from the persisted task, id `bridge-task-start:<taskID>`), `BridgeServiceApplication+Approvals.swift`; UI = plain DOM JS `BridgeDesktopUI/Resources/pages-workbench.js` (`approvals-tray`/`approval-card`/user-input), `pages-native-permissions.js`, `pages-settings.js` | BridgeDomain/BridgeServiceCore/Crypto; WKWebView UI | partial — **decisive**: approvals are presented and decided **only** on the local App/XPC path (ChatGPT/Qwen and Supervisor get no approval tool, `docs/COMPATIBILITY.md`); the UI is presentation-only and the decision semantics live in transport-neutral service actors (cleanly separable), but codex/direct pending approvals are **in-memory** and lost on restart (only task-start approval and the `approval.requested/resolved` events are durable) | 2 (UI only); the decision model is not reused | low (JS renderers) / medium (full shell wiring) | own UI; decisions stay in Policy |
| **Project Registration** | yes | `BridgeProjects/` (`ProjectRegistry.swift` 154, `ProjectModels.swift` 284, `ProjectRepository.swift`, `ProjectExecutionContext.swift` = 562 LOC); `BridgeServiceApplication/ServiceProjectRepositoryAdapter.swift`, `ProjectGitStatus.swift`; UI `pages-projects.js` + `NSOpenPanel` via `BridgeDesktopCommandRouter+Projects.swift` | BridgeAgentCore, BridgeDomain, BridgeSecurity | partial — clean actor API + explicit errors, but `RegisteredProject` is upstream's own domain (`RegisteredRoot` canonical path + **inode** identity, `repositoryRoot ⊇ primaryRoot`, `read/write/network` policy, verification commands, forbidden patterns) and persistence goes through `ServiceProjectService` → `SimpleServiceStore` | 2 (copy with attribution) or not reused | low–medium | own UI; CodeBridge owns the registry |
| **Agent Discovery** | yes | `BridgeServiceHost/ServiceAgentAutoDiscovery*.swift` (~1,550 LOC) + `ServiceAgentDiscoveryCache/Catalog`; `BridgeServiceCore/ServiceAgentRegistry*` (probe/registration, inode+sha256 identity pinning); providers are exactly Qoder, Pi, Antigravity, OpenCode, DeepSeekHarness (`ServiceComposition.swift:140-177`); **Codex is deliberately not a registry provider** | BridgeAgentCore, BridgeProcess, Security | partial — `canonicalRegularFile`, `AgentExecutableResolver`, `userAgentDirectories`/`macOSAgentSearchDirectories` and the bounded DSH source crawl are portable; per-provider detection is hardcoded (CLI names, npm package ids, `/Applications/*.app` paths, Windows PE-subsystem filtering) | 2 (copy with attribution) | medium–high | own detection |
| **Agent Connectivity** | yes | `BridgeAgentCore/AgentProvider.swift:257-279` (protocol: `descriptor`, `probe`, `models`, `start`) + normalised `AgentExecution{Request,Event,Control,Handle}`; adapters `BridgeCodexRPC` (app-server JSON-RPC over stdio), `BridgeACP` + `BridgeOpenCodeACP`/`BridgeDeepSeekHarnessACP` (ACP over stdio), `BridgeAntigravityCLI` (stream-json CLI), `BridgePiRPC` (JSON-record RPC), `BridgeQoderSDK` (Node host script) | Crypto, Process, per-harness CLIs/SDKs (user-installed) | partial — the provider protocol and normalised event/approval/control model are clean and provider-agnostic; each adapter is bespoke and version-coupled (OpenCode ACP range, DSH modern/legacy launch, Qoder `qoder-sdk-v1` pin, Antigravity `--help` fact parsing, Pi extension contract) | 2 per harness (or 3 for a large one) | high | own adapters |
| **SQLite task/session** | yes | `BridgeServiceCore/SimpleServiceStore*.swift` + `ServiceStoreSchema*.swift` (1,649 LOC, `version = 23`, 23 migrations, GRDB `DatabaseQueue`); daemon smoke created `service.sqlite` with 13 app tables (`bridge_service_meta`, `projects`, `tasks`, `task_events`, `task_messages`, `task_queue`, `task_attachments`, `task_usage`, `settings`, `handoffs`, `agent_installations`, `agent_installation_artifacts`, `agent_runtime_artifacts`) + `grdb_migrations`; DB at `~/Library/Application Support/CodexBridgeService/service.sqlite` | GRDB.swift 7.11.1 (`Configuration`: `busyMode .timeout(5)`, `foreignKeysEnabled`; **WAL not enabled** — rollback journal; single serialized writer) | partial — mature transactional schema with optimistic-concurrency writes, integrity checks and pre-migration backups, but it is upstream's domain model and has **no `sessions` table** (session identity lives in nullable `codex_thread_id`/`provider_session_id` task columns); CodeBridge V2 defines its own schema ([Architecture](../architecture.md) §Store) | 2 (patterns) or not reused | medium (low for greenfield; high to open an existing upstream DB) | own schema |

### 6. Build and smoke (observed)

**`swift build` (unchanged, `Packages/BridgeCore`)** — PASS:

```text
$ cd Packages/BridgeCore && swift build
… resolving 9 pins (swift-nio fork, GRDB 7.11.1, swift-log 1.15.0, swift-crypto 3.12.0, …)
… compiling 1674 units
[1642/1674] Compiling BridgeServiceAppShell ServiceRegistration.swift
Build complete! (152.74s)
BUILD_EXIT:0
```

Produces `Packages/BridgeCore/.build/debug/codex-bridge-service` (`Mach-O 64-bit executable arm64`) and the `codex-bridge-windows-app` executable. External deps fetched over the network; a `yeyuancc0-glitch/swift-nio` **fork** is required (no substitution possible without patching).

**Daemon smoke (unchanged binary, isolated temp data root, no TCC/installer)** — PASS:

```text
$ ./.build/debug/codex-bridge-service --foreground --data-root /tmp/cb-smoke4
Codex Bridge service ready on 127.0.0.1:63564.
RESIDENT: yes            # pgrep found the pid after 6 s
SIGTERM: exited gracefully
```

Data root created with mode `0700`; `service.lock`; `service.sqlite` (180,224 B) plus `AgentState/`, `SupervisorScratch/`, `TunnelRuntime/`. Opening the smoke DB read-only shows **13 app tables** + `grdb_migrations` (23 rows = schema v23): `bridge_service_meta`, `_projects`, `_tasks`, `_task_events`, `_task_messages`, `_task_queue`, `_task_attachments`, `_task_usage`, `_settings`, `_handoffs`, `_agent_installations`, `_agent_installation_artifacts`, `_agent_runtime_artifacts`. The daemon binds a **loopback TCP** MCP endpoint (`127.0.0.1:<random>`) and the store is a GRDB `DatabaseQueue` with **rollback journal (no WAL)**; the default data root (without `--data-root`) is `~/Library/Application Support/CodexBridgeService`, guarded by an flock on `service.lock`.

**Documented test command (`swift test`)** — fails by absence, as expected from the filtered public tree:

```text
$ swift test --package-path Packages/BridgeCore
Building for debugging...
Build complete! (8.02s)
error: no tests found; create a target in the 'Tests' directory
SWIFT_TEST_EXIT:1
```

**Documented Xcode app build** — `README_en.md:139-142` / `CONTRIBUTING.md:18` document:

```text
Scripts/with-xcode.sh xcodebuild -project CodexBridge.xcodeproj -scheme CodexBridge \
  -configuration Debug -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath .build/Xcode build CODE_SIGNING_ALLOWED=NO
```

**Result on this workstation (`Observed`, verbatim command):** the destination is accepted and the SwiftPM graph resolves, but the build then **stalls after `CreateBuildDescription` / `ExecuteExternalTool` with zero compile units and no error for 40 minutes** (reproduced in an earlier 60-minute attempt). The `.app` bundle build is therefore **not verified here**; a toolchain mismatch is the likely cause (the project was last upgraded with Xcode 27 — `LastUpgradeCheck = 2700` — while this host runs Xcode 26.4), but that is `[INFERENCE]` and unproven. Source compilation itself **is** proven: the `swift build` above compiles all 30 targets including the macOS-only `BridgeServiceAppShell`. No code signature is available on this workstation (`0 valid identities`), so any app build here is build-only (`CODE_SIGNING_ALLOWED=NO`) and does not exercise signing/notarization.

### 7. Tunnel helper contract and local MCP endpoint (observed, not executed)

`TunnelManager.helperArguments()` (`BridgeTunnel/TunnelManager.swift:353-379`) spawns the pinned helper as:

```text
<doctor|run>
--control-plane.tunnel-id tunnel_<32 lowercase alnum>
--control-plane.api-key=file:/dev/fd/3            # Windows: env:CODEX_BRIDGE_TUNNEL_API_KEY
--mcp.server-url http://127.0.0.1:<port>/mcp
--mcp.extra-headers "X-Codex-Bridge-Token: file:/dev/fd/4"   # Windows: env:CODEX_BRIDGE_TUNNEL_TOKEN
--harpoon.allow-plaintext-http=true
--health.listen-addr 127.0.0.1:0
--health.url-file <run-dir>/health.url
--pid.file <run-dir>/tunnel.pid
--allow-remote-ui=false --open-web-ui=false --log.level warn --log.format json
```

- Secrets ride anonymous fds (3 = Keychain Runtime Key, 4 = 256-bit local MCP header secret), written once and closed; env is emptied to `TMPDIR`+`CODEX_HOME`; stdin is `/dev/null`; the child is `posix_spawn`ed **suspended**, re-verified by live PID, then `SIGCONT`ed (`TunnelProcessLauncher.swift`).
- The local MCP server is a swift-nio `127.0.0.1:<port>` HTTP Streamable endpoint at `/mcp`, authenticated by constant-time `X-Codex-Bridge-Token`, and requires the modern MCP handshake (`Mcp-Protocol-Version: 2026-07-28`, `Mcp-Method`, `Mcp-Name`, `_meta`) for non-initialize traffic (`BridgeMCP/HTTP/*`, `MCPSessionRegistry+Routing.swift`).
- Health is **loopback TCP HTTP** (`/readyz`, `/metrics`) despite the file name `UnixHealthClient.swift`; readiness also requires a fresh control-plane poll metric and that the supervised helper PID owns the listening port (`proc_pidinfo`). `doctor` runs before every `run` and exit 2 is accepted only for an exact `oauth_metadata` 404 (`TunnelDoctorCompatibility.swift`).
- Restart is two-layer: in-process `TunnelManager` never self-restarts (child exit → `failed`), while `ServiceTunnelController` restarts with backoff `[1,2,4]s` then doubling capped at 60 s, unbounded, and deliberately does **not** loop on auth/identity/config failures.

## Conclusion

**PASS for a pin-and-audit Phase 0; upstream is usable behind adapters, but not as an app fork.** The upstream is pinnable and reproducible (`win` @ `7844bb608a9a4e96ed09c084589b7825db77aa3e`, tag `v1.3.4`, Apache-2.0, unchanged `swift build` exit 0, daemon smoke ready + graceful SIGTERM + real SQLite store; the `.app` Xcode build was **not** verified — see §6). It is a **~147k-line single-author agent-orchestration host**: it solves tunnel supervision, local approvals, project registration, agent discovery/connectivity, Keychain and a durable SQLite store well, and it solves **none** of Computer Use.

Three findings dominate the reuse decision:

1. **No Computer Use exists upstream** — CodeBridge's differentiating engine (`computer.*`) has no upstream code to reuse; it must be built by CodeBridge.
2. **The upstream MCP surface and session model are its own** — 42 tools + a web-UI-bound session/approval model. Per the frozen migration rules these are **not** reused; only capabilities go behind CodeBridge-owned adapter protocols (`ApprovalPresenting`, `SecretStoring`, `Updating`, `AgentDetecting`, `ProjectPicking`).
3. **Release identity is ad-hoc, and the project is a fast-moving vendor fork** (365 commits in 7 weeks; a `swift-nio` fork is required to build) with **no root SwiftPM manifest**, so mode 1 (remote package dependency) is unavailable and upstream cannot be tracked as an unpinned package.

Recommended reuse modes, corrected for the constraints above:

- **`BridgeTunnel` + the `[OI]` helper supply scripts** → **mode 3 (vendored subtree with a patch queue)**, consumed locally via a `path` package, because it is security-relevant, pinned to the `tunnel-client` v0.0.10 CLI contract, and expected to need fixes. (Mode 1 is impossible: no root `Package.swift`; mode 2 is a poor fit at ~2.3k LOC.)
- **`BridgeSecurity` Keychain, agent-discovery detection logic, project models, approval UI, updater feed/client** → **mode 2 (copy with attribution)**: small, self-contained, or UI-only.
- **`BridgeAgentCore` provider protocol + per-harness adapters** → **mode 2 per harness** (only the harnesses CodeBridge actually ships); the protocol shape may inform CodeBridge's own `AgentProvider` port but must not be imported as CodeBridge's contract.
- **Packaging/signing scripts** → **mode 2** or rebuild; **notarization** → implement from scratch.
- **Upstream app shell, MCP catalogue, session/approval decision model and store schema** → **not reused**. Mode 4 (fork/run the app) is rejected.

## Architecture impact

- No frozen V2 decision is invalidated; this fills the Phase 0 gap in [Architecture](../architecture.md) §3.3 and [Migration](../migration.md) §4.1/§4.2. D11 ("reuse mode per upstream module") is now answered per module above.
- Upstream's ingress is **loopback TCP `/mcp` with a constant-time header token**, whereas [Architecture](../architecture.md) §3.3/§5.2 describes `codebridged` exposing Streamable HTTP MCP **over a Unix-domain socket** for `tunnel-client`. That is an intentional CodeBridge-side change; the upstream supervisor (`--mcp.server-url`, health/`/readyz`, `proc_pidinfo` port-ownership check, suspended-launch `SecCode` verification) is reusable but its *endpoint binding* must be adapted by CodeBridge.
- Upstream **cannot** be a SwiftPM package dependency: its only manifests live in subdirectories, and SwiftPM remote dependencies require a root `Package.swift`. Any SwiftPM consumption is therefore a **local `path` dependency into a vendored subtree** ([Migration](../migration.md) §4.3 mode 3), which also keeps TCC/identity review under CodeBridge's control.
- Upstream **does** validate the live peer's code signature over local IPC (`BridgeServiceHost/LocalAppPeerIdentity.swift`): the guest is resolved **by PID** (`SecCodeCopyGuestWithAttributes(kSecGuestAttributePid:)`), its path must equal the expected sibling app, and it must satisfy that app's **designated requirement** (`SecCodeCheckValidity`). This matches the Host IPC "app role live peer signature validation" requirement and is a reusable *pattern*, but upstream's transport is `NSXPCConnection(machServiceName:)` (macOS) / a DACL-restricted named pipe (Windows) — **not** a UDS — with a JSON envelope (`schemaVersion = 4`, 75 operations, ≤8 MiB payload) rather than length-prefixed JSON-RPC2. Note the `anonymous` XPC mode disables the app-identity gate (`requiresAppIdentity = false`), so the check is a property of the mach-service path, not a global invariant.
- Upstream's local MCP server enforces the **modern MCP handshake** (`Mcp-Protocol-Version: 2026-07-28`, `Mcp-Method`, `Mcp-Name`, `params._meta`) for non-initialize traffic. CodeBridge should decide its own MCP wire behavior deliberately; this is a compatibility surface that upstream pins and CodeBridge is free to diverge from, but the strictness is a useful reference for caller-class admission.
- Upstream's approvals are **local-path only** (the remote MCP clients get no approval tool) and mostly **in-memory**: pending Codex/provider tool approvals and Direct-action approvals are lost on restart, while the remote task-start approval is *derived* from the persisted task row and therefore survives. CodeBridge V2 wants persisted approval/`grant`/`approval` records and an opt-in approval-only `remote_human` widget path — both are CodeBridge-side divergences; upstream's approval decision surface must **not** be reused as the decision owner (though its separable service-actor structure is a useful reference).
- Signing/notarization: the upstream public artifact is ad-hoc signed and **not** notarized; there is no working notarization implementation to copy — CodeBridge's release identity remains its own workstream.
- Upstream carries **no TCC grants of its own** (empty entitlements, no TCC APIs), which is consistent with [Architecture](../architecture.md) §14's rule that upstream-derived code runs under CodeBridge.app's grants and every sync is a security review.

## Follow-up

- **Before any import (Phase 1+):** run [Migration](../migration.md) §4.6 — add `desktop/macos/ThirdParty/UPSTREAM.md` recording repo/commit/license/modules/mode, keep the Apache-2.0 LICENSE + extended NOTICE, and sync in dedicated commits reviewed as security changes.
- **Ingress spike (already tracked):** validate `tunnel-client` supervision against a **UDS** MCP target, not upstream's TCP default; confirm the health/readiness state machine against the real control plane. `tunnel-client` is absent on this workstation and there are 0 signing identities, so the helper re-signing path is **Untested**.
- **Adapter boundaries:** define CodeBridge-owned `SecretStoring`/`AgentDetecting`/`ProjectPicking` protocols first; upstream types must not cross into Domain or Host IPC.
- **Do not depend on upstream as a package** (not published as one) and do not fork the app (mode 4 rejected). If an upstream commit becomes unreachable or the project stalls, no Phase 1 item depends on upstream ("the upstream decision is recorded, even if it is 'not used'").
- **Safe verification commands for the parent (no TCC, no installer, no system state):** the two commands that produced the PASS results above are safe to re-run —
  `cd /tmp/codex-bridge-upstream/Packages/BridgeCore && swift build` (offline after first resolve; writes only `.build/`), and
  `/tmp/codex-bridge-upstream/Packages/BridgeCore/.build/debug/codex-bridge-service --foreground --data-root "$(mktemp -d)"` under a timeout (binds a random loopback port, writes only the temp data root, exits on `SIGTERM`). Do **not** run `Scripts/build-tunnel-helper.sh` (downloads `tunnel-client`), `Scripts/build-release-candidate.sh` (re-signs a bundle), or any installer.
- Re-verify the pin at import time: `origin/win` had no drift at audit time, but the repository was pushed to within hours of this audit (release published the same day) and a `swift-nio` fork is required to build.
