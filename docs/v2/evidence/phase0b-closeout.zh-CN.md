# Phase 0B — 阻塞关闭 / 真实签名原生验收

日期：2026-10-02。主机：macOS 27.0（26A428），arm64。

**PHASE0_BLOCKED**

- F2：**NOT CLOSED**。
- Phase 1：**NOT READY**。未进入 Phase 1，未启用 Computer input。
- **SIGNED_TCC_BLOCKED_EXTERNAL**：当前 keychain 有效签名 identity 为零。
- **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**：没有可用 Tunnel 凭证/可访问的真实 ChatGPT 验收会话。
- **CONTINGENCY_NOT_PUBLICLY_SUPPORTED**：responsibility disclaimer 没有已建立的受支持公开实现，未加入私有 API/SPI。

[English](phase0b-closeout.md)。历史 [Phase 0 收尾](phase0-closeout.zh-CN.md) 和 [TCC 原始归属](phase0-tcc-attribution.txt) 原样保留，不重写。

## 阻塞、实测证据与最小下一步

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

## 明确停点

先补真实证书/私钥及可访问的真实 ChatGPT/Tunnel 前提，再执行明确 UNTESTED 的签名矩阵，包括 daemon C 直接 native API。不能从编译、unsigned 拒绝 smoke 或历史本地 PASS 推断完成；门槛未开时不继续权限、输入或真实 OS 转换实验。
