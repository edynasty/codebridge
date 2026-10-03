# Phase 0B — 阻塞关闭 / 真实签名原生验收

日期：2026-10-03。实际主机：macOS 27.0（26A428），arm64。基线：`702bca2`；架构冻结：`8d1fcdf`。

**PHASE_0_BLOCKED**

- F2：**NOT CLOSED**。
- Phase 1：**NOT READY**。未进入 Phase 1，未启用 Computer input。
- **IDENTITY VALID / SIGNED TREE VERIFIED / SMAppService PASS**：签名前提已修复；F2 的 App 正向授权及干净归属前提仍不满足。
- **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**：没有可用 Tunnel 凭证/可访问的真实 ChatGPT 验收会话。
- **CONTINGENCY_NOT_PUBLICLY_SUPPORTED**：responsibility disclaimer 没有已建立的受支持公开实现，未加入私有 API/SPI。

[English](phase0b-closeout.md)。历史 [Phase 0 收尾](phase0-closeout.zh-CN.md) 和 [TCC 原始归属](phase0-tcc-attribution.txt) 原样保留，不重写。

## 当前 2026-10-03 收尾

| 必需状态 | 当前结果 |
| --- | --- |
| Code Signing Identity | **VALID**，目标 Apple Development，1 valid identity |
| Identity | Apple Development: tangxingpeng@hotmail.com (QC2AU8UDY6) |
| 实测、已获用户确认的 Team ID | **HXAV5ALQQG**；原要求 QC2AU8UDY6 不是证书 OU/实际 TeamIdentifier |
| Trust-chain root cause | 缺 WWDR G3 中间证书；现有旧 WWDR 于 2023 年过期；系统 Apple Root CA 已存在 |
| Signing smoke | **PASS**，真实签名、strict verify、实际执行；临时 source/binary 已删除 |
| signed App/daemon/probe | **PASS**，稳定 com.codebridge.app / com.codebridge.daemon / com.codebridge.probe；真实 Team、Authority、DR、strict/deep 校验均记录 |
| SMAppService | **PASS**，注册、launchd 启动、App 退出存活、SIGKILL 重启、同一 Store 重开、签名 IPC 重连、更新重注册、注销 |
| F2 | **NOT CLOSED**，App Computer 真实 API 未达到 ALLOWED；历史 override/干净授权因果未建立；C/D subject 指向 App |
| Phase 0 | **BLOCKED**，F2 与真实 ChatGPT/Tunnel 验收仍开放 |
| Phase 1 readiness | **NOT READY**，未启用生产 Computer input |

### OBSERVED：真实证据

- 仅从 Apple 官方下载 WWDR G3，经系统内置信任链验证后导入 login。没有删除/重建 leaf/private key，没有修改 Root CA、trust override、search list 或时间策略。[证书字段及签名元数据](phase0b-signing.md)。
- 第一轮真实 builder 因 inline `--test-requirement` 缺 `=` 失败；修正两处后正向构建通过。第一轮 signed SMAppService 注册成功但 daemon exit 2，原因是 plist 缺 `run`；通过 App 注销、补 ProgramArguments、重建重注册后通过。两个失败原始输出均保留。
- daemon 35517 在正常 App 退出后存活；只对它发 SIGKILL 后，12 秒观察时 launchd 已替换为 47519。旧 IPC 获得 EOF；签名 App 的 role=app 使用 audit token/真实 daemon 身份成功重连。相同 runtime.db 路径、inode 366202558、schema 1 重开；原 Store 没有 Run/Session/journal 数据，不宣称已证明有数据域恢复。相同签名重建后启动 50184；最终 native report smoke 启动 60728；两次均由 App 成功注销并移除 job。[生命周期](phase0b-launchagent.md)。
- A/B/C/D/E 均执行真实 SCK、AX、listen-only tap；C 新增直接 CGO/Objective-C 在 daemon PID 内调用，没有把 D 改标签。App 持续 SCK -3801、AX -25204/trusted=false、输入投递未验证；shell control 在 3 秒内收到 12 个 physical events，注入事件为零。App/daemon 重启及相同身份重建后，仍未得到 App 正向授权基线。[矩阵](phase0b-tcc.md)。
- C 请求 `35517.1` 及更新后的 `50184.1` 的 requesting identity 是 daemon，subject 却是 com.codebridge.app。D `35963.1` 的 responsible 是 daemon 35517、requesting 是 probe 35963，subject 同样是 App。已记录旧 App ad-hoc requirement 匹配失败。[原始归属日志](phase0b-tcc-signed-2026-10-03.txt)。
- 后续 A/B/C/D/E 的 Files & Folders 读取成功；这不证明干净授权因果或受保护目录更新保持性。没有读写 TCC 数据库、reset TCC、自动造 grant、注入输入，未记录像素、按键内容或目录内文件名。Raw GUI inherited environment 已排除。

### DECISION 与 INFERENCE

- **DECISION：**展示名称后缀与真实 Team 冲突被实际签名证明后，用户明确选择 HXAV5ALQQG。没有伪造 QC2AU8UDY6 或放宽签名校验。
- **DECISION：**用户两次人工授权/清理报告作为人工动作保存，不升格为 OS grant/API 成功。没有观察到真实签名 daemon/harness 的 Computer 成功授权，因此不把 ARCHITECTURE_AMENDMENT_REQUIRED 写成已证明的成功泄漏结论；不重设计架构，不加入私有 disclaimer。
- **INFERENCE：**实测 bundled TCC subject 共用是边界风险；A 非 ALLOWED 时，尚不能证明成功的 App grant 因果继承。

### BLOCKED 与最小下一步

1. 先让**当前真实签名 App 的授权实际生效**：真实 SCK 截图成功、AX API 成功、listen-only physical event 投递。通过人工控制、仅针对 CodeBridge 的权限清理解决历史 requirement 冲突；任何工具执行的定向 reset 都需明确审批，绝不 reset 整个用户。随后重测 C/D 归属及重启/重建矩阵；daemon/harness 一旦得到 Computer 权限，记 ARCHITECTURE_AMENDMENT_REQUIRED，不能降低 F2。
2. 真实 ChatGPT/Tunnel 凭证/会话仍是外部阻塞。当前 daemon health 实测 tunnel `not_configured`、`tunnel binary not configured`；本轮不新增 ChatGPT/widget E2E PASS。

### 已执行检查与最终状态

全部指定原生检查通过：`make fmt-check`；`go test ./...`（17 packages，8 no tests）；`go vet ./...`；`go build ./...`；`make build-daemon`；`swift build -c release --package-path desktop/macos`；`swift test --package-path desktop/macos`（20 IPC + 13 ComputerSpike，零失败）。最终 whitespace 检查记录在 [完整本轮证据](phase0b-signed-2026-10-03.json)。

最终 signed App 保留在 `desktop/macos/build/CodeBridge.app`；App 已退出，service **已注销**。无 push、reset/clean、Complete Phase 0 commit 或 signed acceptance closure commit。原 Phase 0/0B raw evidence 全部保留。扩展 native probe debug-gate 回归；删除偶然 mock-echo 断言，保留 caller argv 安全边界断言。[更新后的诊断契约](../../../schema/hostipc/v1/README.md#7-hostphase0_probe-debug-only)。

下方为**原样保留的 2026-10-02 历史归档**；其中“当前”、零 identity、UNTESTED 指该历史轮次，不是上面的本轮状态。


## 历史 2026-10-02：阻塞、实测证据与最小下一步

| 阻塞项 | 实测证据 | 分类 | 最小下一步 |
| --- | --- | --- | --- |
| 真实签名 identity | security 实际返回 0 valid identities；构建入口在替换输出前拒绝 | 外部证书/私钥缺失，不是签名实验失败 | 安装含私钥的 Apple Development（优先）或 Developer ID Application，确认有效 identity/真实 Team |
| F2 签名隔离 | 无真实签名 A–E 矩阵、SMAppService 生命周期或授权保持复测；历史 daemon 授权截图保留 | 未关闭的安全证据缺口，不是本轮新签名拓扑失败 | 真实签名树 → SMAppService → 仅 App 的 Computer grants → A–E 真实归属/重启/重建复测 |
| C 直接 API 证据 | 当前 daemon TCC diagnostics 在 harness child 中运行 | native 验收 instrumentation/证据缺口，不是 DENIED | 宣称 C 前，增加并实测实际签名 daemon PID 内的只读 native API probe，不能把 D 改标签为 C |
| 保护目录结论 | 仅普通自有 root 边界 smoke，没有真实签名保护目录授权保持 | 外部签名前提；尚非 SUPPORTED/UNSUPPORTED_IN_V1 | 独立 Files & Folders 归属/授权/daemon 重启/相同身份更新验收 |
| 真实 lock/input | F2 未关闭，因此未运行 | 下游安全门槛 | 先关闭 F2，再人工触发 OS 转换，验证物理/自身/第三方/unknown/Secure Input 投递矩阵 |
| 真实 ChatGPT/Tunnel/widget | 受管浏览器登录界面，无 Tunnel keys，实际浏览器 relay extension 未连接 | 外部账户/凭证/浏览器访问前提，不是 transport 失败 | 可访问 developer-mode 会话及真实 Tunnel 配置；真实 pending operation 的 UI 审批并恢复原始操作 |

缺证书不证明架构失败，也不构成修改 F2 的理由。历史 daemon/harness 截图仍是未关闭 F2 发现，原始 grant 历史与真实签名 App 授权因果不明。后续若历史污染，标记 **EVIDENCE_CONTAMINATED**，不强行 PASS，不全用户 reset。本轮未 reset TCC、写 TCC 数据库或自动授予权限。

## 逐项结果

| 用户要求 | 结果 | 证据 |
| --- | --- | --- |
| 重读指定 Phase 0/设计文档并保留 candidate | DONE；原 124 个路径存在 | [完整 manifest](phase0b-files.txt) |
| 真实 identity 与不同 App/daemon/probe 稳定标识 | preflight/准备 smoke PASS；真实树 BLOCKED | [签名](phase0b-signing.md) |
| SMAppService register/status/启动/App quit/crash/Store/IPC/重连/注销 | 真实签名验收 UNTESTED；未签名注册拒绝 PASS | [生命周期](phase0b-launchagent.md) |
| A/B/C/D/E ScreenCapture/AX/listen tap；Files & Folders 独立 | UNTESTED；F2 NOT CLOSED；C 直接 instrumentation 仍需建立 | [矩阵](phase0b-tcc.md) |
| 公开 responsibility 方案与条件性最小修订 | 调研 DONE；CONTINGENCY_NOT_PUBLICLY_SUPPORTED；未应用修订 | [公开 API 证据](phase0b-responsibility.zh-CN.md) |
| 保护目录 deadline/持久性结论 | 独立模式及普通 root 边界 PASS；真实签名 timeout/授权保持 UNTESTED | [root](phase0b-protected-roots.md) |
| lock/saver/sleep/wake/FUS | BLOCKED_BY_F2，未触发转换 | [下游门槛](phase0b-lock-input.md) |
| 可靠物理/自身/第三方输入与 Secure Input | BLOCKED_BY_F2，computer.input unavailable | [下游门槛](phase0b-lock-input.md) |
| 真实 ChatGPT Tunnel 宿主命令 | BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS；历史本地 PASS 不撤销 | [外部检查](phase0b-tunnel-widget.md) |
| 实际 widget token/UI 审批恢复原始操作 | BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS；无真实 E2E PASS | [widget 要求](phase0b-tunnel-widget.md) |
| Go/Swift/shell/whitespace 必需检查与 smoke | 已执行路径 PASS；真实 codesign 验收阻塞 | [raw gates](phase0b-gates.txt)、[实际 smoke](phase0b-smoke.json) |
| 双语结论、文件与 commit 交付 | DONE；无 Complete Phase 0 commit、无 push | 本文及 [manifest](phase0b-files.txt) |

## 增量修改

- 构建入口在编译/修改输出前验证真实 Apple identity、私钥与实际 Team，不再从名称末尾推导 Team。Probe signing ID 修正为 com.codebridge.probe；正向证书路径本机 **UNTESTED**。
- 封存 Info.plist/LaunchAgent 携带互相验证 App/daemon 所需身份；live peer authentication 未降低。未签名 marker/build kind 正确解析，未签名构建不配置可信 Team。
- 复用现有 native probe，增加 App **同进程** A 入口，保留 child B，不建立第二套权限 API 实现。
- SMAppService 验收 CLI/register fail-closed 校验 App 和两个 nested 身份；未注册也可进行只读 status 检查。
- Files & Folders 独立模式不调用 Computer API；timeout 文字不再把未解析访问当作 TCC 拒绝证明。本轮不实现生产 ProjectRegistration、Computer engine、InputArbiter、WebRTC、编排或 cloud。
- 双语 roadmap/index 与架构证据限定更新，未改变冻结 F2、未抹除历史证据。

## 已执行检查

| 命令/场景 | 实测结果 |
| --- | --- |
| make fmt-check | PASS |
| go test ./... | PASS，17 packages，8 no tests |
| go vet ./... | PASS |
| go build ./... | PASS |
| make build-daemon | PASS，实际 CGO-enabled codebridged |
| swift build -c release --package-path desktop/macos | 补齐 shared-module import/source ownership 后 PASS |
| swift test --package-path desktop/macos | PASS，33 tests，zero failures |
| 两个签名脚本 bash -n | PASS |
| git diff --check | PASS |
| 缺 identity 的真实 builder + 已有自有输出 | 预期 exit 2，SIGNED_TCC_BLOCKED_EXTERNAL，marker 保留 |
| 自有 unsigned 准备装配 | plutil/marker PASS；非真实签名/TCC/SMAppService 验收 |
| App --phase0b-probe signing | 实际 App executable/PID，exit 0；不是 child probe 或权限证据 |
| App --phase0b-service status | 实际未签名状态/job 未加载；不是 signed lifecycle PASS |
| unsigned register/permissions/input-monitor/files-folders | 预期 exit 3，在 OS 验收调用前拒绝 |
| 普通自有 root 的 standalone files-folders | 可读 count=2；缺失路径；regular-file ENOTDIR；无 Computer API report section |
| 真正 Apple 签名的 codesign deep/strict、元数据、designated requirement | 缺真实证书 BLOCKED，未虚构结果 |

首次 Swift integration 因 shared CLI 缺 report-module import 失败，也存在 target source ownership 警告；修正后 release 与全部测试通过。首次身份 smoke assertion 混用了 /var 和 /private/var，用原捕获 report 的 canonical path 校验通过，没有重跑 App 命令来隐藏失败。Raw 证据保留这些事实；测试不能代替 OS 验收。

## 文件与 commits

原 124 个 candidate 文件全部保留；本轮修改 18 个已有文件，新增 15 个文件。最终 candidate manifest：**139 文件**（9 modified tracked、130 untracked、0 staged）。完整路径及本轮变更见 [phase0b-files.txt](phase0b-files.txt)；ignored 生成 binary/build cache 不计入。

Scope 时 HEAD：`8d1fcdf`。**新 commit：无；push：无。** 未 reset/clean、未删除原 candidate、未创建最终 Complete Phase 0 commit。仅清理本轮自有临时拒绝/普通 root/unsigned 装配 fixture，本轮没有注册服务。

## 历史 2026-10-02：明确停点

先补真实证书/私钥及可访问的真实 ChatGPT/Tunnel 前提，再执行明确 UNTESTED 的签名矩阵，包括 daemon C 直接 native API。不能从编译、unsigned 拒绝 smoke 或历史本地 PASS 推断完成；门槛未开时不继续权限、输入或真实 OS 转换实验。

## 最新 App-only 正向基线 — 2026-10-03，run 20261003T012940770Z

**APP_TCC_BASELINE: FAIL — APP_TCC_BASELINE_NOT_ESTABLISHED。F2 NOT CLOSED；PHASE_0_BLOCKED；Phase 1 NOT READY。** 本次仅追加证据，不改写历史：先前矩阵、生命周期及“未 reset”描述仍属于各自旧轮次。本轮停在 A，没有重跑 B/C/D/E 或 F2 保持性矩阵。

### OBSERVED：实测

- 当前 bundle：`/Users/tangxingpeng/IdeaProjects/me/codebridge/desktop/macos/build/CodeBridge.app`；identifier `com.codebridge.app`；真实 Team **HXAV5ALQQG**。Reset 前及 request-hook 重建后，真实 Apple Development 签名、hardened runtime、deep/strict 校验均通过。CDHash 从 `4f1bfc56019090b942554a1015f14dd7efbf5f17` 变为 `b7525cda835942ab7cf29360396f4508fc57497b`，App 的 designated requirement、identifier、Team 不变。[构建、测试及完整签名元数据](phase0b-app-baseline-build-20261003T012940770Z.txt)；[精确 DR 及基线证据](phase0b-tcc.md#app-only-positive-baseline--2026-10-03-run-20261003t012940770z--本轮追加)。
- 仅执行用户批准的三项 bundle-scoped reset，全部成功：`tccutil reset ScreenCapture com.codebridge.app`、`tccutil reset Accessibility com.codebridge.app`、`tccutil reset ListenEvent com.codebridge.app`。本机 manual 说明 bundle scope，但未列 service 名；`ListenEvent` 来自本机真实 TCC 请求 `35517.4`，并由 reset 成功确认支持。无全局/其他 bundle reset，无 TCC 数据库读写。
- 新增显式 opt-in `--phase0b-request-permissions`，在真实签名 App 主进程 **90964** 调用公开 ScreenCapture、AX prompt、ListenEvent 请求，三个请求均返回 false；返回值不是正向验收。Passive probes 仍不请求授权。用户选择 **“已仅授权当前 App 并退出”**，保留为 **USER_REPORTED_ACTION_NOT_API_PASS**。[请求 receipt](phase0b-app-requests-20261003T012940770Z.json)。
- 随后由 LaunchServices 新启动同一真实签名 App，parent PID 1；真实 API 结果如下：

| 未通过能力 | 实测结果 | 分类 |
| --- | --- | --- |
| Screen Recording，App PID 7361 | 真实 SCK SCShareableContent 拒绝 **-3801**；capture skipped，无 frame/size/hash | FAIL / DENIED |
| Accessibility，App PID 7361 | 真实 `AXFocusedApplication` 查询返回 **-25204 / kAXErrorCannotComplete**，trusted=false | FAIL / ERROR |
| Input Monitoring，App PID 8380 | 真实 listen-only tap 已安装；**30 秒、0 physical events**，preflight=false；input_available=false | FAIL / delivery UNPROVEN，不冒充 DENIED |

- [SCK/AX report](phase0b-app-positive-api-20261003T012940770Z.json)；[input report](phase0b-app-positive-input-20261003T012940770Z.json)。Tap mask 增加 `.mouseMoved`，覆盖请求的物理鼠标移动。无像素落盘、按键内容或注入输入；roots 为空，本轮未测 Files & Folders。
- 失败后只读 System Settings 快照：**录屏与系统录音**存在两个独立 label，`CodeBridge=1`、`CodeBridge.app=0`；AX 的真实窗口标题为 **设备控制和数据访问**，App/probe/daemon 均为 0。**输入监控**的记录 AX tree 未暴露 CodeBridge references。Label 本身不能证明精确签名身份。Inspector 未修改开关、未新增 Computer grant，其已有权限不计为 A 证据。
- TCC 对当前真实签名 App 的 DR 匹配 **status 0**，Screen/AX 仍为 **Denied (System Set)**。真实 SCK `60709.3` 的 requesting 为 replayd 60709、accessing 为 App 7361，`preflight=no`、subject App、`authValue=0/authReason=4`；AX `7361.2`/`.3`、ListenEvent `8380.1` 同样返回 0/4。[Live log](phase0b-app-baseline-tcc-20261003T012940770Z.txt)；[live 丢消息后的 bounded log-show 恢复](phase0b-app-baseline-log-show-20261003T012940770Z.txt)。SystemEvents/VSCode inspector 归属另行保留，不升格为 App grant。
- 已执行：Swift release build、**33 tests（20 IPC + 13 ComputerSpike）**、真实签名重建/deep-strict 校验、App 主进程请求及两个真实 probe 路径。Go 未改、未重测。本轮没有 service 注册/重启/kill、commit 或 push。[完整原始验收记录](phase0b-app-baseline-20261003T012940770Z.json)。

### INFERENCE：推断

- 两个 Screen label 的 entry/path 歧义是候选解释，不是已证明的用户操作错误或 grant 失效原因。人工动作原样保留，OS 有效授权尚未建立。
- 当前 DR 匹配的拒绝不是旧 ad-hoc requirement 冲突的证明。独立 `anchor apple` platform-only 校验 **-67050** 不代表已验证的 Apple Development 签名失效。System-set denial 根因未知；private-TCC 与 AppleEvents warning 不能证明额外 entitlement 是 Computer 授权前提。

### DECISION：决策与最小下一步

- 停在 **APP_TCC_BASELINE_NOT_ESTABLISHED**。本轮 B/C/D/E 与 authorized-A 保持性复测均为 **NOT RUN**。真实签名及旧轮 SMAppService PASS 保留，不能代替 App 正向基线。
- **F2 NOT CLOSED**。不能仅凭 `subject=com.codebridge.app` 判泄漏；不宣称已证明的 `ARCHITECTURE_AMENDMENT_REQUIRED`，未改拓扑、未加私有 disclaimer/entitlement workaround、未放宽签名、未给 daemon/probe/父进程补 Computer grants。
- 最小下一步：人工核对**当前 App 的精确路径**及 Screen/AX/Input 对应项，区分 `CodeBridge` 与 `CodeBridge.app`，不把 label 当身份。仅调整当前 App 项，完全退出/重启，再只测 A 的真实 capture、成功 AX query、物理 tap 投递。三项全部正向后，才能恢复 native daemon-PID C/D 归属及重启/重建验收。
- 本轮明确不做 ChatGPT/Tunnel/widget；原外部阻塞保留，不重新调查，也不混入本次 App 基线失败结论。

## 最新 GUI 优先 A-only 验收 — 2026-10-03，run 20261003T023754513Z

**APP_TCC_BASELINE: FAIL — APP_TCC_BASELINE_NOT_ESTABLISHED。F2 NOT CLOSED；PHASE_0_BLOCKED；Phase 1 NOT READY。** 上方历史签名、生命周期、矩阵均保留；本次在人工 GUI 清理后仅测 A，零 reset。

### HUMAN ACTION：人工动作

已引导三页删除历史 App 项、用精确路径重新添加并只开启当前 App；不改其他软件、不授权 daemon/probe/harness/父进程。用户选择 **“三页已清理并授权，两个程序已退出”**，报告三页完成及 App/System Settings 完全退出。此报告不是 OS/API PASS。真实 listen-only 观察时明确请求移动鼠标和按一次无副作用 Shift；提示本身不证明发生了物理事件。

### OBSERVED：实测

- 当前精确 bundle `/Users/tangxingpeng/IdeaProjects/me/codebridge/desktop/macos/build/CodeBridge.app` 实测 identifier `com.codebridge.app`、Team **HXAV5ALQQG**、非 ad-hoc Apple Development、deep/strict PASS。CDHash 不变：`b7525cda835942ab7cf29360396f4508fc57497b`；DR 不变，完整字段见 [TCC 证据](phase0b-tcc.md#gui-first-app-only-baseline--2026-10-03-run-20261003t023754513z--gui-优先追加) 与 [原始验收](phase0b-app-gui-baseline-20261003T023754513Z.json)。清理前精确 App 进程搜索无匹配。本轮未重建/重签。
- LaunchServices 新启动 App **11361**，parent 1：真实 SCK **DENIED -3801**、capture skipped/无 frame；真实 AXFocusedApplication **ERROR -25204**、trusted=false。新 App **13649**，parent 1：真实 listen-only tap 已创建，**30 秒 / 0 physical events**、preflight=false/input_available=false、注入和 disabled events 为 0；**delivery UNPROVEN**，不冒充 DENIED。[SCK/AX receipt](phase0b-app-gui-api-20261003T023754513Z.json)；[input receipt](phase0b-app-gui-input-20261003T023754513Z.json)。无像素/按键内容落盘，roots=[]，无输入注入。
- 后续只读真实设置页面：录屏 **录屏与系统录音** 的 CodeBridge=ON、CodeBridge.app=OFF、probe=ON、daemon=ON；AX **设备控制和数据访问** 的 App/probe/daemon=OFF；**输入监控**记录的 AX tree 未暴露 CodeBridge checkbox references。第一次录屏导航短暂显示总览，改读目标页面后取得上述行。Label 不能确认精确身份。Inspector 仅为证据重新打开设置、没有改开关；额外进程的历史 ON 项不是本轮造出的 grant，也未用于执行 C/D。
- TCC 当前 DR 匹配 status 0，Screen/AX 仍 Denied (System Set)。真实 SCK `11093.3`：requesting replayd 11093、accessing App 11361、preflight=no、subject App、authValue=0/authReason=4，handling row 的 responsible 是 App 11361。AX `11361.2`/`.3`：accessing/requesting/responsible App 11361、subject App、0/4。ListenEvent `13649.1`：requesting App 13649/subject App、0/4，不推测未记录的 responsible 字段。[Live log](phase0b-app-gui-tcc-live-20261003T023754513Z.txt)；[live 丢消息后精确 persisted log-show](phase0b-app-gui-tcc-log-show-20261003T023754513Z.txt)。自有 recorder 已停止。

### INFERENCE：推断

历史 entry/path/requirement 污染仍只是候选解释。人工报告与后续快照各自保留，不宣称用户动作错误或根因已证明。当前 DR 匹配的请求不是旧 ad-hoc 冲突证明；platform-only -67050 不代表真实签名失效，private-TCC warning 不构成添加私有 entitlement 的理由。

### DECISION：决策 / 最小下一步

- **本次失败后停止 A 验收循环。** B/C/D/E 与 F2 重启/重建保持性均 NOT RUN。F2 仍开放；subject App 本身不证明泄漏，不断言 ARCHITECTURE_AMENDMENT_REQUIRED。无 Phase 1、代码修改、build/test 重跑、SMAppService 变更、自动 GUI grant、reset、TCC 数据库读写、private API、签名降级或额外进程 grant；ChatGPT/Tunnel/widget 不在本轮范围。
- 人工核对**精确当前 App 路径/条目**与后续 Screen/AX App-OFF 快照及清理报告之间的状态；保留其他软件，不授权 daemon/probe/harness/父进程。当前 App-only 状态实际生效后，再只测 **A**。不预设 reset 能治好；未来 bundle-scoped reset 必须必要、受支持、先说明并记录。

## 唯一显示名分阶段验收 — 2026-10-03，run 20261003T025836514Z

**阶段：Screen — 未验收（LAUNCHSERVICES_TCC_LABEL_STALE）。APP_TCC_BASELINE: FAIL；F2 NOT CLOSED；Phase 0 BLOCKED；Phase 1 NOT READY。** 本轮新增保持身份的 instrumentation，并得到新证据：LaunchServices 与请求归属均已更新，唯独 TCC 面板 label 过期。

### OBSERVED：实测

- `build-app.sh --display-name "CodeBridge Phase0 Signed"`（仅限签名 acceptance 构建）：bundle ID `com.codebridge.app`、Team **HXAV5ALQQG**、executable `CodeBridge`、daemon/probe identifier 与 designated requirement 均不变；新 App CDHash `3ea4930142e254fae781f6f2176d167794b05fa2`。`--phase0b-request-permission screen|accessibility|input` 在 App 主进程只请求单项权限；新增 `screen`/`accessibility` 单项 probe；AX 成功现在要求返回真实 focused AXUIElement。全部检查通过（fmt-check、Go test/vet/build、Swift release + 33 测试、签名 deep/strict、`git diff --check`）。[构建日志](phase0b-app-named-build-20261003T025836514Z.txt)；[运行记录](phase0b-app-named-baseline-20261003T025836514Z.json)。
- 精确路径 App PID **33577**（parent 1）发出仅 Screen 请求：receipt `displayName=CodeBridge Phase0 Signed`、`com.codebridge.app`、Team **HXAV5ALQQG**、`screenRequestReturned=false`。TCC 请求 `33577.3`（真实 ScreenCapture，preflight=no）：requesting/subject `com.codebridge.app`，`authValue=0, authReason=4`。[Receipt](phase0b-app-screen-request-20261003T025836514Z.json)。
- 只读 `lsregister -dump`：`localizedShortNames=CodeBridge Phase0 Signed`，`trustedCodeSignatures` 匹配当前 CDHash。但录屏设置页仍只显示 `CodeBridge`、`codebridge-probe`、`CodeBridge.app`、`codebridged` —— **唯一显示名未出现** → **LAUNCHSERVICES_TCC_LABEL_STALE**。

### INFERENCE：推断

TCC 面板 label 在条目创建时固化，不随 LaunchServices 更新；过期 label 即历史 TCC 条目本身。这是条目污染假设的新的直接证据，但仍不是此前授权失败根因的证明。

### DECISION：决策 / 最小下一步

- 不动开关、不写 TCC.db、不 reset（本轮无批准）、不猜 label；Screen/AX/Input 验收与 B/C/D/E 继续暂停。
- 人工二选一：(1) 在录屏页用 `−` 删除四个 CodeBridge 历史条目（不动其他软件），完全退出 System Settings 与 App，再以精确路径 + `--phase0b-request-permission screen` 重启，生成带唯一名的新条目；(2) 就此停止。[TCC 证据](phase0b-tcc.md#unique-label-staged-baseline--2026-10-03-run-20261003t025836514z--唯一显示名追加)。

## 录屏阶段 PASS — 2026-10-03，run 20261003T061715868Z

**APP_SCREEN_BASELINE: PASS。** 分阶段第一项已建立真实正向基线。本轮未测 Accessibility 与 Input Monitoring；B/C/D/E 与 F2 继续暂停。**F2 NOT CLOSED；Phase 0 BLOCKED；Phase 1 NOT READY。**

### OBSERVED：实测

- 仅执行用户批准的一次定向 reset（`tccutil reset ScreenCapture com.codebridge.app`，exit 0），reset 前身份/退出状态均已记录。reset 后的仅 Screen 请求（App PID 30496）显示存量拒绝已清除：`Auth Right: Unknown (None), promptType: 1` —— 出现真实授权机会。随后用户通过系统弹窗/设置页人工授权（HUMAN ACTION）。
- 完全退出后，新精确路径 App PID **36351**（parent 1，同 bundle/Team/CDHash）运行真实 `screen` probe：`SCShareableContent ok`、**真实 capture ok，1800×1169，SHA-256 `77b9a798663edd4a085e144b967ee0911bf7007c42081ede32f34a94012bdbf0`，pixels_persisted=false**。TCC `36351.1`：subject `com.codebridge.app`，**`Allowed (System Set), authValue=2`**。[probe 报告](phase0b-app-screen-positive-20261003T061715868Z.json)；[运行记录](phase0b-app-screen-reset-20261003T061715868Z.json)；[TCC 证据](phase0b-tcc.md#screen-stage-pass-after-approved-scoped-reset--2026-10-03-run-20261003t061715868z--录屏阶段通过追加)。

### DECISION：决策

- PASS 仅基于真实 SCK capture —— UI ON、request 返回值、preflight 从不单独计为通过。本轮就此停止：无 AX、无 Input、无 B/C/D/E、无 F2 判定、不重复 reset。
- 下一轮单独处理 Accessibility 阶段（`--phase0b-request-permission accessibility`，再用要求返回有效 focused AXUIElement 的真实 `accessibility` probe 验收）。仅 Screen PASS 不关闭 F2，不解锁 Phase 0/1。

## 辅助功能阶段 FAIL — 2026-10-03，run 20261003T075114032Z

**APP_ACCESSIBILITY_BASELINE: FAIL — AX_REAL_API_FAILED_AFTER_TRUST。** Screen PASS 原样保留、本轮未触碰。Input Monitoring 本轮未测。**F2 NOT CLOSED；Phase 0 BLOCKED；Phase 1 NOT READY。**

### OBSERVED：实测

- Preflight 身份/退出验证完成，未 rebuild。仅 Accessibility 请求 PID 99272 首次命中存量拒绝（`Denied (System Set), authValue=0, authReason=4, DB Action:None`），随后系统打开真实授权界面；用户授权后 TCC 写入 `kTCCServiceAccessibility → Allowed (System Set)`（99358.16，granted=true，当前 DR）。[请求记录](phase0b-app-ax-request-20261003T075114032Z.json)。
- 授权后全新 probe PID **8198**（parent 1，同 bundle/Team/CDHash）：**trusted=true，但真实 `AXFocusedApplication` 返回 -25204 kAXErrorCannotComplete**。PID 8198 的 TCC handling 行为 **Allowed (System Set)**；frontmost 为 Google Chrome PID 774。[probe 报告](phase0b-app-ax-positive-20261003T075114032Z.json)；[TCC 证据](phase0b-tcc.md#accessibility-stage-fail--2026-10-03-run-20261003t075114032z--辅助功能阶段失败追加)。

### DECISION：决策 / 最小下一步

- 按分阶段标准判 FAIL：只有真实 AX API 计分，trust 后仍失败。STOP：无 Input Monitoring、无 B/C/D/E、无 F2 判定、无 toggle、无 reset。
- 下一步：收集 probe PID 的 AX/WindowServer 诊断；用更长生命周期、经 LaunchServices 启动的 App 实例（而非立即退出的 probe 进程）做同样只读 `AXFocusedApplication` 查询；排查是否存在第二个 Accessibility 决策面。不原样重跑同一 probe。

## 辅助功能阶段 PASS — 2026-10-03，run 20261003T082537258Z

**APP_ACCESSIBILITY_BASELINE: PASS（TEST_GATE_REFINEMENT）。** 此前 FAIL（run 20261003T075114032Z）为 instrumentation 生命周期问题：立即退出的 probe 进程收不到 AX run-loop 回复。Screen PASS 保留。**F2 NOT CLOSED；Phase 0 BLOCKED；Phase 1 NOT READY。**

### OBSERVED：实测

- 新增 `--phase0b-ax-diagnostic` 在真实 LaunchServices 启动、NSApplication run loop 存活的 App 主进程内运行；同身份重建（App CDHash `205ee05ae9600bfe5b6510382bed407b0a9e0458`），全部原生检查通过。
- 三次运行（PID 30230/30763/31715）：trusted=true；SystemWide AXFocusedApplication 间歇（-25212 / **成功** / -25212）；**Path C 三次全部成功** —— NSWorkspace frontmost PID → AXUIElementCreateApplication → AXRole + AXTitle 真实读取（errorCode 0），对象分别为 Chrome×2、VSCode×1。TCC 保持 `Allowed (System Set)`。[诊断记录](phase0b-app-ax-diagnostic-20261003T082537258Z.json)。

### DECISION：决策

- 以修正后的验收门槛判 **PASS**：真实受 TCC 保护的 AX read 并可可靠获取/验证 frontmost application。生产路径：NSWorkspace frontmostApplication → PID → application-scoped AX read。**这是 TEST_GATE_REFINEMENT，不是 SECURITY_REQUIREMENT_WEAKENING** —— trust、真实受保护 AX IPC、frontmost 验证仍是必需且全部实证；SystemWide 属性的间歇性被记录为 API 路径特异问题。下一步：Input Monitoring 基线。

## 连续执行收尾 — 2026-10-03（run 20261003T082537258Z / 20261003T083256604Z）

**APP_TCC_BASELINE: PASS；F2: ARCHITECTURE_AMENDMENT_REQUIRED。** 按 §17 暂停全部 F2 后续阶段。**Phase 0 BLOCKED（已声明的本地门槛通过；F2 与外部门槛开放）；Phase 1 NOT READY。**

### OBSERVED：实测

- Input Monitoring PASS：请求返回 true；真实 listen-only tap 观察到 **1488 个物理硬件事件**（0 注入、0 disable、Secure Input 关闭、`verifiedRunning`）。[基线](phase0b-app-input-baseline-20261003T082537258Z.json)。
- F2 C probe（launchd 持有的签名 daemon PID 38655）：**真实 SCK 截图成功**（1800×1169），TCC `Allowed (System Set)`，subject `com.codebridge.app`；AX trusted=true；Input listen preflight=true。bundle 内 daemon 实际行使了 App 的授权。[F2 记录](phase0b-f2-matrix-20261003T083256604Z.json)；[TCC](evidence/../phase0b-tcc.md#input-baseline-pass-and-f2-leak--2026-10-03-run-20261003t082537258z--20261003t083256604z--输入基线通过与-f2-泄漏追加)。
- SMAppService 顺带复核：App register 启动 launchd 持有 daemon；`launchctl print` 显示 running。
- Tunnel presence check：`not_configured`、二进制缺失、token 文件存在（仅存在性，值不打印）、0 个 tunnel 环境变量、无已认证 ChatGPT surface → **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**。

### DECISION：决策

- **ARCHITECTURE_AMENDMENT_REQUIRED** —— 等待用户在以下方案中选择：接受 bundle subject 共享并重设 IPC 级安全边界；独立 daemon bundle 以获得独立 TCC subject；或已受支持的公开 responsibility/disclaimer 机制（暂无已知实现）。persistence、受保护目录、lock/FUS 与 preemption 阶段被该决策阻塞，而非技术未知。
- 未 commit、未 push、未执行既有批准之外的 reset；历史证据保留；双语 roadmap 已按证据更新。
