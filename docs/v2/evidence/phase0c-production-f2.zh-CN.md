# Phase 0C — 生产独立 Daemon 与 F2 关闭 / Production Independent Daemon & F2 Closure

日期：2026-10-03。**F2: CLOSED。** 拓扑：`INDEPENDENT_DAEMON_PRODUCTION`。[完整矩阵记录](phase0c-production-f2-20261003T104731270Z.json)；[spike 历史](phase0c-independent-daemon-tcc.zh-CN.md)。

## 架构

```
CodeBridge.app（TCC 特权 Computer host）
  │ 安装/更新/控制（DaemonInstaller，SecStaticCode 校验 payload）
  ▼
~/Library/Application Support/CodeBridge/bin/codebridged（外部、已签名）
  ▼ launchd 用户 LaunchAgent（~/Library/LaunchAgents/com.codebridge.daemon.plist）
launchd → codebridged（PPID 1）
```

## OBSERVED —— 实现

- `build-app.sh` 现在从**已签名**的嵌套 daemon（`codesign --identifier com.codebridge.daemon` 之后）复制到 `Contents/Resources/DaemonPayload/` —— payload 携带真实签名（CDHash `7fe900be…`、Team `HXAV5ALQQG`），绝非未签名的 Go 产物。最初的未签名 payload 缺陷被安装校验发现并在任何 launchd 使用前修复。
- `DaemonInstaller.swift`（App）：payload/staging/目标三处 `SecStaticCode`/`SecCodeCheckValidity` 校验、staging→安装原子替换、plist 生成（`plutil -lint`）、严格 argv 的 `launchctl` 封装（超时）、回滚。菜单提供 install/uninstall。
- `guard.go` + native `cb_daemon_passive_tcc`（daemon）：`DAEMON_COMPUTER_TCC_MUST_BE_NONE` 不变量 —— 仅 passive 检查（CGPreflight/AXIsProcessTrusted/CGPreflightListen），绝不请求/弹窗；运行边界调用 `ComputerTCCBoundaryCheck()`；health 报告 `computer_tcc.safe` 并在违规时降级。**已实测**：launchd 持有的 daemon 报告 `safe=true`（三项 preflight 全 false）；shell 环境下的 daemon 正确报告 unsafe 并记录 `UNSAFE_DAEMON_COMPUTER_PERMISSION`。
- 生产 gate 已验证：无 `CODEBRIDGE_PHASE0_DEBUG` 时 `host.phase0_probe` 返回 `-32601 unsupported`（矩阵经 plist env 开启 gate 后运行）。
- Host IPC peer auth 现在拒绝 executable 仍在 App bundle 内的已验证 daemon（遗留 bundled 拓扑）。

## OBSERVED —— 生命周期验收

| 步骤 | 结果 |
| --- | --- |
| 安装（校验 → staging → 原子替换） | PASS |
| launchd bootstrap 与启动 | PASS |
| PPID = 1 / 外部 realpath | PASS（PID 54848/57832/58599/60758） |
| Health + Host IPC | PASS（status ok，store wired） |
| SIGKILL → launchd 重启 | PASS（54848 → 57832） |
| Store 同一 runtime.db 重开 | PASS（schema 1） |
| 更新（原子替换 + kickstart） | PASS（→ 58599，身份保持） |
| App 退出存活 | PASS（daemon 跨越多个无 App 运行时段） |
| DAEMON_COMPUTER_TCC_MUST_BE_NONE | PASS |

## OBSERVED —— 干净 F2 矩阵（仅 App 授权，daemon 无授权）

| | Screen | AX | Input | TCC subject |
| --- | --- | --- | --- | --- |
| **A** App | **ALLOWED**（真实截图 1800×1169，重建后 PID 65521） | **ALLOWED**（trusted） | **ALLOWED**（preflight true） | com.codebridge.app |
| **C** 外部 daemon PID 60758，进程内 | **DENIED**（−3801） | **DENIED**（trusted=false） | **DENIED**（preflight=false） | 外部 daemon 自身路径 |
| **D** probe 子进程 PID 64234（parent 60758） | **DENIED** | **DENIED** | **DENIED** | 外部 daemon 路径（responsible=外部 daemon，绝非 com.codebridge.app） |

Phase 0C 的 D-Screen 污染已解决：生产环境干净条件下 **D-Screen = denied**。

## OBSERVED —— 持久性

- App 重启/重建：App 重签后 A 保持 ALLOWED（Screen capture ok、AX trusted、Input preflight）；C/D 全程 DENIED。
- Daemon SIGKILL 重启与原子更新：身份与拒绝保持（矩阵在重启/更新后的 PID 上运行）。
- **F2_PERSISTENCE: PASS。**

## DECISION

**F2: CLOSED。** 外部独立 daemon 建立了 OS 强制的 Computer TCC 边界：只有 CodeBridge.app 持有 Computer 授权；daemon 及其子进程在三项服务上全部拒绝，subject 为自身路径。已记录（未 reset）的残留：已删除的 bundle 路径与 Phase0C 路径 daemon 条目的过期 Screen 授权仍在 TCC 中；无害（路径已删除），用户可手动移除。

## Phase 0 后续

F2 关闭 → 下一个门槛：protected roots、lock/saver/sleep/FUS、input preemption（各需人工动作），然后是外部 ChatGPT/Tunnel。Phase 0 仍因外部凭证 **BLOCKED**；本地门槛已推进。
