# Phase 0 收尾 — 2026-10-02

**PHASE_0_BLOCKED — Phase 1 NOT READY。** 架构冻结于 `8d1fcdf`；未修改冻结的拓扑与信任决策。[English](phase0-closeout.md)。

## Phase 0 Summary

已实现并运行最小 Go daemon、原生 Swift App/probe、UDS MCP ingress、Host IPC 协议、Durable Store 和显式 Capability 定义。V1 Manager/Client/RunManager/RunStore/适配器保持不变，仓库级 Go 检查通过。本轮不是完整 Computer Use engine、scheduler、审批 presenter 或媒体实现。

**F2 安全门槛未关闭：** 真实 launchd daemon 的子进程成功截取一帧；现有 TCC 日志把该 ScreenCapture 请求归属到 daemon。已停止 Computer 权限与输入实验。真实签名隔离、签名受保护目录授权保持、真实 ChatGPT/Tunnel 审批仍缺前置条件，不能延期到 Phase 1 再处理。

## Checklist

下表 `FAIL` 表示必需验收证据缺失或安全门槛未关闭，不等于冻结架构已被证明不可实现。仅 Task 10 按 Roadmap 明确的 Phase 2 门槛允许 DEFERRED。

| 用户 Task | 结果 | 实际证据 / 未满足验收 |
| --- | --- | --- |
| 1 — 上游审计 | **PASS** | `Fanch-hui/codex-bridge`，`win`，tag `v1.3.4`，SHA `7844bb608a9a4e96ed09c084589b7825db77aa3e`；Apache-2.0 LICENSE/NOTICE 与完整 12 行 Capability Matrix。原样 SwiftPM core build、真实 service/SQLite smoke 通过；上游无 test target；完整 `.app` Xcode build 停滞，**未验证**。审计 PASS 不代表 App 构建认证。未导入上游源码或类型。[审计](phase0-upstream.md)。 |
| 2 — Secure MCP Tunnel | **FAIL / BLOCKED** | 真实 UDS MCP initialize/tools/ping、Bearer caller classes、真实 tunnel-client 托管的本地路径通过。没有 control-plane tunnel ID/API key；实际 ChatGPT/Platform browser 要求登录。真实 ChatGPT→Tunnel 往返、在线 Tunnel 恢复与断连隔离未验证。[Tunnel](phase0-tunnel.md)。 |
| 3 — daemon 生命周期 | **FAIL / BLOCKED** | 手动 per-user launchd fixture：ppid 1，崩溃重启 62447→72321，Store 重开、Bearer 轮换、正常 bootout。没有有效签名身份/Team ID，真实签名 `SMAppService` 注册及注册后 App 退出存活未验证。[生命周期](phase0-launchagent.md)。 |
| 4 — 原生宿主 | **PASS** | 真实 launchd daemon 执行 shell/git/Docker Desktop **Server**/SSH/kubectl，全部 exit 0；记录 argv、stdout/stderr、PATH/HOME/USER、cwd `/`。未经过 Manager 或容器执行。[Native host](phase0-native-host.md)、[机器结果](phase0-parent-smoke.json)。 |
| 5 — 签名 TCC 隔离 | **FAIL — ARCHITECTURE_BLOCKER F2** | daemon 72321 的真实子进程 77878 在内存中成功截图 1800×1169。现有 TCC 请求 `77878.1` 的 responsible 与 authorization subject 均是 daemon，authValue 2。**未证明**真实签名 App 授权继承的因果关系；原始授权来源未知。实验已停止。[TCC](phase0-tcc.md)、[原始归属日志](phase0-tcc-attribution.txt)。 |
| 6 — 受保护 Root | **FAIL / BLOCKED** | Desktop/Documents/Downloads 最初在 launchd 下达到读取时限，后续可读；普通 `/tmp` 可读。缺真实签名的 rebuild/update 授权保持对照。不能诚实选择 `SUPPORTED` 或 `UNSUPPORTED_IN_V1`；该必需发布决策仍阻塞，不是“可能支持”。[TCC §7](phase0-tcc.md#7-parent-integration-f2-stop-condition)。 |
| 7 — lock/saver/sleep/wake/FUS | **FAIL / BLOCKED** | 实际 locked 初始快照使模型 suspended、controller none、epoch/geometry 失效，并丢弃一个已入队模型动作。测量窗口无真实切换；缺 OS capture/injection/队列重放切换证据和第二用户 GUI session。已纠正此前被拒绝的队列 seed 指标；模型测试不能替代 OS 证据。[Lock](phase0-lock-state.md)。 |
| 8 — Input Monitoring | **FAIL — 正向可行性未验证** | 已实测 listen-only tap；Secure Input active 时强制 unavailable。可靠 physical/第三方/自身注入区分及抢占未验证。`computer.input` 保持 unavailable；未注入输入。F2 阻止进一步依赖实验。[Input](phase0-input-monitor.md)。 |
| 9 — Approval-only widget | **FAIL / BLOCKED** | 真实 UDS 跨 HTTP 请求决策、token 只在 `_meta`、once/session、拒绝 replay/missing/wrong-widget/project/always、不创建 Grant；本地 HTML 已运行。没有登录的真实 ChatGPT widget，无法证明模型隐藏/UI-only host 执行与 host widgetSessionId 两种情况。[Widget](phase0-widget-approval.md)。 |
| 10 — WebRTC 可行性 | **DEFERRED — 仅因 Phase 2 门槛允许** | 真实本地 HTML 的 RTC offer/answer/data channel 通过；真实 ChatGPT Web/Desktop/Mobile/CSP/STUN/TURN 未验证。不能免除 Task 9。仅在真实 widget 不可行得到证明后考虑冻结的 stateless rendezvous fallback。[媒体](phase0-webrtc.md)。 |
| 11 — Host IPC v1 | **PASS — 最小协议** | 语言无关 Schema、framing、hello/version/capabilities/health，真实 Go↔Swift UDS 握手。0700 父目录/0600 sockets、peer UID/audit identity、live Apple-anchored signature 拒绝、版本/方向/角色负路径通过。真实签名 App 正向仍属独立身份/TCC 阻塞；不声称实现了 Computer/approval/notify provider 方法。[IPC](phase0-ipc.md)。 |
| 12 — Store schema v1 | **PASS** | 必需实体与 schema_version ledger、Application Support 默认路径、WAL/FULL/FK、备份后 forward migration、拒绝新版 Schema、原子 state+event、seq/pos 连续；事务/reopen smoke 和迁移回归通过。未导入 V1 DB。[Store](phase0-store.md)。 |
| 显式 Capability | **PASS — 仅定义** | Computer/Agent 必需 flags 包含 false、五个 Phase 1 tool Schema 与 Policy/visibility 要求。JSON Schema 验证及缺 flag 负例通过。不从 OS/provider type 推断，不声明任何 Computer 工具可用。[Capabilities](phase0-capabilities.md)。 |

## Architecture Blockers

### F2 — Computer 权限隔离未建立

Assumption：daemon/harness 不得通过 App/父级责任归属取得 Computer TCC 权限。Environment：macOS 27.0 (26A428)、arm64、UID 501、ad-hoc binaries，无真实 Team ID。Steps：真实 launchd daemon 执行固定原生权限 probe；发现意外截图成功后，仅**只读检查现有 unified logs**。

Observed：ScreenCapture 最初拒绝 `-3801`，后续截图成功（1800×1169；仅保存 SHA-256）。请求 `77878.1` 的 responsible PID **72321**、identifier **a.out**、subject **bin/codebridged**；authValue **2**、authReason **4**、无 error。未保存像素、按键内容或受保护目录内文件名。原始 launchd GUI 环境和 Bearer/token 值未进入仓库证据。

Conclusion：**ARCHITECTURE_BLOCKER F2**。这是实际 daemon 归属的截图，**不是**真实签名 App 授权因果继承的证明，也**不是**冻结双进程拓扑不可行的证明。原始授权来源未知。不能用 ad-hoc 的拒绝/成功结果批准真实签名的生产隔离边界。

Required minimum architecture amendment/containment：使用**冻结文档已有的 F2 应急措施**——独立签名的 daemon 身份与 disclaimed-responsibility harness spawning，然后执行受控的真实签名 App/daemon/harness 授权对照，任何 Computer input 发布前必须通过。未重设架构、自动授权/reset、弱化签名校验或增加推测 fallback。已 bootout 自有 LaunchAgent，daemon 日志记录 shutdown requested/stopped。所有依赖的 Computer 权限与输入实验继续停止。

其他阻塞前置条件：真实 Developer ID 身份 + Team ID；人工操作 lock/saver/sleep/wake 与第二用户会话；登录的 ChatGPT developer mode、workspace 关联 tunnel ID/runtime API key。实际浏览器登录检查、环境/配置检查、签名身份检查没有取得这些条件。必须真实测试关闭门槛，本地替代不能作为验收。

## Tests 与真实 smoke

| 实际执行命令 / 场景 | 结果 |
| --- | --- |
| `make fmt-check`；`gofmt -l schema/store/v1` | PASS，无未格式化文件 |
| `go test ./...` | PASS；17 个测试 package，8 个无测试 package；包含 V1 |
| `go vet ./...`；`go build ./...`；`make build-daemon` | PASS；daemon 启用 cgo 的 live Security framework 校验 |
| `go run ./cmd/store-smoke` | PASS；非法终态迁移拒绝、stream 连续、controller event、reopen seq 6 / pos 9 |
| `swift build -c release --package-path desktop/macos` | PASS |
| `swift test --package-path desktop/macos` | PASS；**33 tests**，零失败 |
| 三个 `desktop/macos/Building/*.sh` 的 `sh -n`、`bash -n` | PASS |
| `build-app.sh --unsigned-dev --daemon bin/codebridged --embed-phase0-env` | PASS，开发 bundle/plist 组装；明确**不是**真实签名/TCC/SMAppService 证据 |
| `git diff --check` | PASS |
| LaunchAgent 真实 daemon + IPC + native host + SIGKILL restart + bootout | 本地生命周期/传输/native-host 子集 PASS；签名 App 注册仍阻塞 |
| MCP Go SDK、直接 UDS HTTP、跨请求审批 | 本地传输/token 状态子集 PASS；真实 Tunnel/ChatGPT 阻塞 |
| 真实 tunnel-client 托管 | 观察到 flags/config 校验、有限次数重启、干净退出；**未建立** control-plane 连接 |
| 实际权限 probe / 现有 TCC 日志 | **观察到 F2 blocker**；签名隔离未验证 |
| Lock/Input probe | 实际 locked snapshot、Secure Input 负路径；真实切换/抢占未验证 |
| 真实本地 widget browser / RTC data channel | 本地面 PASS；真实 ChatGPT widget 未验收 |
| 上游原样 core build / service smoke | PASS；`swift test` exit 1（无 target）；完整 App Xcode build 未验证 |

未把源码文本/措辞/wiring-only Swift 测试当作安全证据保留。回归覆盖 token 消费、跨请求状态、授权/版本、Store constraints/rollback/migrations、suspension/frame 不变量。未添加完整 injector 或真实媒体 transport。

## Commits

**本轮未创建 Commit，未 Push。** 独立里程碑以 working-tree 修改保留；不把阻塞证据包装为 Phase 0 完成。未 reset/clean 或删除 V1。


最终证据检查：**124 个清单文件均存在**、JSON 均可解析、**148 个相对 Markdown 链接可解析**；Markdown 尾随空白检查通过。先保存脱敏证据、确认 endpoints/token files 已不存在，再删除自有且已停止的 smoke fixture；最终 `git diff --check` 通过。
## Remaining P2 — 仅 Non-blocking

- 真实 host 的 `openai/widgetSessionId` 可用性未知；未收到 server-side binding 时，spike 依赖 token possession。冻结门槛明确允许登记此 Residual Risk；没有扩大 scope。
- 真实 Widget Web/Desktop/Mobile media、CSP、受限网络 STUN/TURN，仅按 **Phase 2 gate** 延期。

签名 TCC、受保护 Root 结论、真实 lock/FUS 安全和 Phase 1 真实审批/入口是**阻塞项**，不是 P2 剩余。输入可行性仍未验证且保持 unavailable；原始 freeze P2 标签不允许发布未经证明的输入。

## Final

```text
PHASE_0_BLOCKED
Phase 1 readiness: NOT READY
Blocking assumption: F2 Computer TCC isolation is not established;
                    signed native and real ChatGPT/Tunnel gates remain open.
Required architecture amendment: frozen F2 signing/responsibility contingency,
                                 followed by genuine signed isolation evidence.
```

## Files Changed

以下是完整 working-tree 文件清单；不含生成 binary、SwiftPM cache、开发 bundle 或自有临时 fixture。
```text
Makefile
cmd/codebridged/main.go
cmd/store-smoke/main.go
deploy/launchd/io.github.edynasty.codebridged.plist.example
desktop/macos/.gitignore
desktop/macos/Building/Info.plist.in
desktop/macos/Building/build-app.sh
desktop/macos/Building/check-signing-identity.sh
desktop/macos/Building/com.codebridge.daemon.plist.in
desktop/macos/Building/smoke-native.sh
desktop/macos/Package.swift
desktop/macos/Sources/CodeBridgeApp/AppDelegate.swift
desktop/macos/Sources/CodeBridgeApp/DaemonClient.swift
desktop/macos/Sources/CodeBridgeApp/LaunchAgentController.swift
desktop/macos/Sources/CodeBridgeApp/MenuBarController.swift
desktop/macos/Sources/CodeBridgeApp/ProbeChildRunner.swift
desktop/macos/Sources/CodeBridgeApp/main.swift
desktop/macos/Sources/CodeBridgeComputerSpike/InputClassifier.swift
desktop/macos/Sources/CodeBridgeComputerSpike/ProbeReport.swift
desktop/macos/Sources/CodeBridgeComputerSpike/SuspensionModel.swift
desktop/macos/Sources/CodeBridgeIPC/Framing.swift
desktop/macos/Sources/CodeBridgeIPC/HostIPCClient.swift
desktop/macos/Sources/CodeBridgeIPC/HostIPCPaths.swift
desktop/macos/Sources/CodeBridgeIPC/JSONRPC.swift
desktop/macos/Sources/CodeBridgeIPC/JSONValue.swift
desktop/macos/Sources/CodeBridgeIPC/PeerIdentity.swift
desktop/macos/Sources/CodeBridgeIPC/PeerTrust.swift
desktop/macos/Sources/CodeBridgeIPC/ProcessIdentity.swift
desktop/macos/Sources/CodeBridgeIPC/ProtocolVersion.swift
desktop/macos/Sources/CodeBridgeIPC/ShellCommand.swift
desktop/macos/Sources/CodeBridgeIPC/UnixSocket.swift
desktop/macos/Sources/codebridge-probe/AccessibilityProbe.swift
desktop/macos/Sources/codebridge-probe/AsyncBridge.swift
desktop/macos/Sources/codebridge-probe/FilesFoldersProbe.swift
desktop/macos/Sources/codebridge-probe/HostEnvironment.swift
desktop/macos/Sources/codebridge-probe/HostIPCHandshakeProbe.swift
desktop/macos/Sources/codebridge-probe/HostToolsProbe.swift
desktop/macos/Sources/codebridge-probe/InputMonitorProbe.swift
desktop/macos/Sources/codebridge-probe/LockStateProbe.swift
desktop/macos/Sources/codebridge-probe/ProbeOptions.swift
desktop/macos/Sources/codebridge-probe/ProbeRunner.swift
desktop/macos/Sources/codebridge-probe/ScreenCaptureProbe.swift
desktop/macos/Sources/codebridge-probe/main.swift
desktop/macos/Tests/CodeBridgeComputerSpikeTests/SuspensionSpikeTests.swift
desktop/macos/Tests/CodeBridgeIPCTests/HostIPCWireTests.swift
docs/v2/README.md
docs/v2/README.zh-CN.md
docs/v2/architecture.md
docs/v2/architecture.zh-CN.md
docs/v2/evidence/phase0-capabilities.md
docs/v2/evidence/phase0-closeout.md
docs/v2/evidence/phase0-closeout.zh-CN.md
docs/v2/evidence/phase0-input-monitor.md
docs/v2/evidence/phase0-ipc.md
docs/v2/evidence/phase0-launchagent.md
docs/v2/evidence/phase0-lock-state.md
docs/v2/evidence/phase0-native-host.md
docs/v2/evidence/phase0-parent-smoke.json
docs/v2/evidence/phase0-store.md
docs/v2/evidence/phase0-tcc-attribution.txt
docs/v2/evidence/phase0-tcc.md
docs/v2/evidence/phase0-tunnel.md
docs/v2/evidence/phase0-upstream.md
docs/v2/evidence/phase0-webrtc.md
docs/v2/evidence/phase0-widget-approval.md
docs/v2/migration.md
docs/v2/migration.zh-CN.md
docs/v2/roadmap.md
docs/v2/roadmap.zh-CN.md
internal/approvalspike/probe.go
internal/approvalspike/probe_test.go
internal/approvalspike/widget.html
internal/daemon/config.go
internal/daemon/daemon.go
internal/daemon/daemon_test.go
internal/daemon/exec.go
internal/daemon/hostipcapi.go
internal/daemon/ingress.go
internal/daemon/mcpclient.go
internal/daemon/mcpserver.go
internal/daemon/probe.go
internal/daemon/smoke.go
internal/daemon/store.go
internal/daemon/store_runtime.go
internal/daemon/tunnel.go
internal/hostipc/client.go
internal/hostipc/errors.go
internal/hostipc/frame.go
internal/hostipc/frame_test.go
internal/hostipc/hello.go
internal/hostipc/jsonrpc.go
internal/hostipc/methods.go
internal/hostipc/peer_darwin.go
internal/hostipc/peer_other.go
internal/hostipc/server.go
internal/hostipc/server_test.go
internal/hostipc/sigcheck_darwin.go
internal/hostipc/sigcheck_other.go
internal/runtime/entities.go
internal/runtime/entities_ops.go
internal/runtime/events.go
internal/runtime/events_test.go
internal/runtime/migrate.go
internal/runtime/migrate_test.go
internal/runtime/paths.go
internal/runtime/policy_ops.go
internal/runtime/runs_ops.go
internal/runtime/store.go
internal/runtime/store_test.go
internal/runtime/transitions.go
internal/runtime/tx.go
schema/capabilities/v1/provider.schema.json
schema/capabilities/v1/tools.json
schema/hostipc/v1/README.md
schema/hostipc/v1/approval.json
schema/hostipc/v1/computer.json
schema/hostipc/v1/envelope.json
schema/hostipc/v1/hello.json
schema/hostipc/v1/host.json
schema/hostipc/v1/methods.json
schema/hostipc/v1/notify.json
schema/hostipc/v1/runtime.json
schema/store/v1/schema.go
schema/store/v1/schema.sql
```

