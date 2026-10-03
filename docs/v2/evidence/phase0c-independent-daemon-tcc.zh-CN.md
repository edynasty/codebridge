# Phase 0C — 独立 Daemon TCC 边界验证 / Independent Daemon TCC Spike

日期：2026-10-03。Run `20261003T084313615Z`。结果：**INDEPENDENT_DAEMON_TCC_BOUNDARY: PASS；F2_TOPOLOGY_SPIKE: PASS_WITH_D_SCREEN_CONTAMINATION（D 的 Screen 行是在用户对外部 daemon 自身条目显式授予录屏后取得的——不是 App 授权继承；干净的 D Screen=DENIED 行必须在生产 F2 矩阵中补回）；架构修正（方案 b）RECOMMENDED。**

修正是实测结论而非理论风险：bundle 内拓扑中，launchd 持有的 `com.codebridge.daemon` 曾以 `Allowed (System Set)`、授权 **subject com.codebridge.app** 真实截屏（[F2 泄漏](phase0b-f2-matrix-20261003T083256604Z.json)、[TCC](phase0b-tcc.md)）。本 spike 验证物理位于 `CodeBridge.app` 之外的 daemon 是否摆脱该归属。

## 实验设置

- 先注销 bundled SMAppService job（避免双 daemon）；App 的 Screen/AX/Input 正向基线未 reset、未重新授权。
- 外部安装：`~/Library/Application Support/CodeBridge/Phase0C/bin/codebridged`（realpath 在 `CodeBridge.app` 外，已验证）。签名不变：identifier `com.codebridge.daemon`、Team `HXAV5ALQQG`、CDHash `eb401909883fd5f502a5600be6e5f7a4a9355603`、同一 DR、strict/deep PASS。
- 独立用户 LaunchAgent `com.codebridge.phase0c.daemon`（`~/Library/LaunchAgents/`），`launchctl bootstrap gui/501`；`KeepAlive.SuccessfulExit=false`、`RunAtLoad`、phase0 debug env 指向外部 probe 副本。Daemon PID **47186、PPID 1** —— launchd 持有，进程树恰为 `launchd → 外部 codebridged`。这是经手动 launchctl 的 Phase 0 spike，不是生产生命周期验收。

## OBSERVED —— C（外部 daemon 进程内）

首探测窗口（16:43:14），外部 subject 尚无任何授权：

| 能力 | 真实结果（daemon PID 47186，`execution=daemon_in_process`） | TCC |
| --- | --- | --- |
| Screen | `SCShareableContent` **denied −3801**，capture skipped，preflight false | `Unknown (None)` → 为外部路径写入 **Denied (System Set)** |
| AX | `trusted=false`，−25204 | `Unknown (None)`；无信任继承 |
| Input | `preflight_listen_access=false`，tap created，0 events | `Unknown (None)` |

**每个请求的 TCC subject：`/Users/…/Phase0C/bin/codebridged` —— daemon 自身路径，绝非 `com.codebridge.app`。** 同一日志窗口并列可见 App 的授权记录（`auth_value=2, com.codebridge.app`），未用于任何 daemon 请求。

## OBSERVED —— 授权异常

16:43:27（C 与 D 之间），`SecurityPrivacyExtension`（System Settings，PID 48048）执行 `TCCAccessSetInternal` → **`Update Access Record: kTCCServiceScreenCapture for 外部路径 to Allowed (System Set)`**。**HUMAN ACTION（用户确认）：C probe 的 SCK 调用触发系统弹窗，用户点了允许。** 这是对外部 daemon 自身 subject 的 Settings UI 授权 —— 不是 App 授权继承，不是脚本写入。

## OBSERVED —— D（外部 daemon 的 probe 子进程）

签名 `codebridge-probe` PID 48501，parent 47186，`launch_context=codebridged_child`：

| 能力 | 真实结果 | TCC |
| --- | --- | --- |
| Screen | capture ok（1800×1169）—— **来自用户在 16:43:27 对外部条目的显式授权** | subject = 外部 daemon 路径，`Allowed (System Set)` |
| AX | `trusted=false`，−25204 | denied |
| Input | `preflight_listen_access=false` | denied |

归属：responsible `com.codebridge.daemon pid 47186`（外部路径）、requesting `com.codebridge.probe pid 48501`、subject 外部路径。任何地方都没有 `com.codebridge.app` 归属。

## 判定

- **INDEPENDENT_DAEMON_TCC_BOUNDARY: PASS** —— 授权前，外部 launchd 持有的 daemon 以自身路径 TCC subject 被三项 Computer 服务全部拒绝。无 App 授权继承。
- **F2_TOPOLOGY_SPIKE: PASS_WITH_D_SCREEN_CONTAMINATION** —— D 不再继承 App 授权（responsible/subject = 外部 daemon，绝非 com.codebridge.app），且 D 会继承外部 daemon *自身*的授权；D 的 Screen Allowed 行源于用户 16:43:27 对外部条目的显式允许，因此在隔离判定上视为被污染。AX 与 Input 对 C、D 全程 denied；授权前的 Screen-DENY 状态已在 C 直接观测。仅 App 授权前提下的干净 D Screen=DENIED 行仍需由生产 F2 矩阵补回。外部 daemon 的 Computer 授权本身就是危险配置——已固化为生产不变量 `DAEMON_COMPUTER_TCC_MUST_BE_NONE`。
- **INFERENCE** —— 外部路径 + launchd 持有是 TCC subject 的 OS 级决定因素；仅签名 identifier（com.codebridge.daemon）不是。spike 还暴露一条生产要求：daemon 侧 SCK 调用会触发真实用户弹窗，因此 daemon 绝不能请求 Computer 权限，IPC policy 必须对 daemon 侧 Computer 能力 fail closed。
- **DECISION** —— **ARCHITECTURE_AMENDMENT_V2_F2 RECOMMENDED**（方案 b）。SMAppService 生命周期验收转为 **HISTORICAL_PASS_FOR_BUNDLED_TOPOLOGY**；外部拓扑需要全新生命周期验收（安装、launchd 启动、App 退出存活、SIGKILL 重启、IPC 重连、store 重开、更新、卸载）与完整 F2 矩阵 + persistence 复测后才可 CLOSED F2。

生产生命周期方向：按用户、无 root、launchd 持有的外部签名 daemon + 用户 LaunchAgent；安装/更新器另行设计；不假设 daemon 走 SMAppService，不用 Manager/Docker。

## 原始证据

[完整 spike 记录](phase0c-independent-daemon-20261003T084313615Z.json)（身份、plist、TCC 行、判定）。运行期间捕获 live TCC 流；异常后的录屏设置页快照显示五个 CodeBridge 条目、两个 `codebridged` 行均 ON（bundle 历史项 + 新外部授权项）。
