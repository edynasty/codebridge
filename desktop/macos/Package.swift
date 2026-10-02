// swift-tools-version:5.9
//
// CodeBridge.app (native host adapter) — Phase 0 skeleton.
//
// Targets:
//   CodeBridgeIPC            language-neutral Host IPC client (framing + JSON-RPC 2.0 + handshake)
//   CodeBridgeComputerSpike  input-free suspension / conservative-input models (spike, no injection)
//   CodeBridgeApp            menu-bar app: LaunchAgent registration + Host IPC handshake + probes
//   codebridge-probe         standalone permission/lock/input harness, runnable from the app,
//                            from the daemon as its child, or from a shell
//
// Contract: docs/v2/provider-contracts.md §6–§7, docs/v2/architecture.md §5, §13.5.

import PackageDescription

let package = Package(
    name: "CodeBridgeMac",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .library(name: "CodeBridgeIPC", targets: ["CodeBridgeIPC"]),
        .library(name: "CodeBridgeComputerSpike", targets: ["CodeBridgeComputerSpike"]),
        .executable(name: "codebridge-probe", targets: ["codebridge-probe"]),
        .executable(name: "CodeBridge", targets: ["CodeBridgeApp"]),
    ],
    targets: [
        .target(
            name: "CodeBridgeIPC",
            path: "Sources/CodeBridgeIPC"
        ),
        .target(
            name: "CodeBridgeComputerSpike",
            dependencies: ["CodeBridgeIPC"],
            path: "Sources/CodeBridgeComputerSpike"
        ),
        .target(
            name: "CodeBridgeNativeProbe",
            dependencies: ["CodeBridgeIPC", "CodeBridgeComputerSpike"],
            path: "Sources/codebridge-probe",
            exclude: ["main.swift"]
        ),
        .executableTarget(
            name: "codebridge-probe",
            dependencies: ["CodeBridgeNativeProbe"],
            path: "Sources/codebridge-probe",
            exclude: [
                "AccessibilityProbe.swift", "AsyncBridge.swift", "FilesFoldersProbe.swift",
                "HostEnvironment.swift", "HostIPCHandshakeProbe.swift", "HostToolsProbe.swift",
                "InputMonitorProbe.swift", "LockStateProbe.swift", "ProbeCLI.swift",
                "ProbeOptions.swift", "ProbeRunner.swift", "ScreenCaptureProbe.swift",
            ],
            sources: ["main.swift"]
        ),
        .executableTarget(
            name: "CodeBridgeApp",
            dependencies: ["CodeBridgeIPC", "CodeBridgeComputerSpike", "CodeBridgeNativeProbe"],
            path: "Sources/CodeBridgeApp"
        ),
        .testTarget(
            name: "CodeBridgeIPCTests",
            dependencies: ["CodeBridgeIPC"],
            path: "Tests/CodeBridgeIPCTests"
        ),
        .testTarget(
            name: "CodeBridgeComputerSpikeTests",
            dependencies: ["CodeBridgeComputerSpike"],
            path: "Tests/CodeBridgeComputerSpikeTests"
        ),
    ]
)
