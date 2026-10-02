# Kimi K3 架构审查记录 — 2026-10-02

状态：**审查完成；整改已落档；最终 Freeze 仍待最后 Gate**

Reviewer：**Kimi K3**  
审查模式：Opus 5.5 架构修订后的独立 Challenge  
范围：当前 CodeBridge V2 工作树；架构、安全、进程边界、Computer Use、Runtime、上游复用、Phase 0 完整性

## 1. 原始结论

本轮审查结论：

```text
NOT READY_TO_FREEZE
```

没有 P0，并实际列出了 **6 个 P1**：

1. 远程场景在 Phase 1 没有人工审批路径；
2. `local_ui` 把“人在场”作为事实，但没有证明经过认证的人类手势；
3. daemon 对 macOS TCC 保护 Project Root 的访问规则缺失；
4. Pointer/App Scope 在真正注入时仍有 TOCTOU 缺口；
5. `type` / `keypress` 没有明确的键盘焦点作用域校验；
6. Phase 0 没覆盖受保护 Root TCC 身份，以及锁屏/screen saver/fast user switching 的真实行为。

> 备注：原始审查的 Verdict 文本写了 “Five P1s”，但紧接着实际枚举的是 1.1、2.1、3.1、4.1、4.2、8.1 共 **6 个 P1**。本记录按实际枚举数量记为 **6 P1**。

审查还登记了若干 P2，但按照审查规则没有要求为 P2 修改架构。

## 2. P1 处置

### K1 — 远程审批缺口

**问题：** Phase 1 只允许本地 UI 审批，但产品同时面向 Secure MCP Tunnel 远程使用。未命中的 `ask` 会让远程启动的 ComputerSession 卡住，直到用户回到 Mac 前。

**结论：****接受。**

**整改：**

- 将最小 `remote_human` 纯审批路径提前到 Phase 1；
- 不依赖 WebRTC；
- 必须显式启用；
- 仅允许 `once` / `session`；
- 禁止 `project` / `always`；
- 禁止 `secret.read` / `cloud.upload`；
- 审批 token 只通过 tool-result `_meta` 下发；
- 模型可见路径仍然永远不能审批。

涉及：

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

### K2 — `local_ui` 人在场假设

**问题：** 仅凭 App 签名和 IPC 身份不能证明真正由人完成审批。

**结论：****接受，并明确残余风险边界。**

**整改：**

- `local_ui` 必须来自 CodeBridge.app 前台审批 UI 中的显式用户操作；
- v1 **不声称**每次审批都经过 LAContext / Touch ID；
- v1 信任边界明确为“已验证 CodeBridge.app + 显式 UI 手势”；
- App 进程被攻陷属于明确记录的残余风险；
- 未来可对特定高风险 Grant 增加更强的人在场认证，不改变 Policy 模型。

涉及：

- `architecture.md` / `architecture.zh-CN.md`

### K3 — Daemon 受保护 Root TCC

**问题：** 当 Project 位于 `~/Documents`、`~/Desktop`、`~/Downloads` 或其他 TCC 保护目录时，如果 `codebridged` 没有自己的稳定 Files-and-Folders 授权，运行时可能只出现不透明的宿主错误。

**结论：****接受。**

**整改：**

- Phase 0 增加 per-user LaunchAgent 的受保护 Root TCC 实测；
- 验证重建/更新后授权是否稳定；
- Project 注册时必须主动探测 Root；
- 缺少宿主/TCC 权限统一映射：
  `permission_denied: host_permission_required`；
- 如果无法形成稳定 daemon 授权，v1 明确不支持这些受保护 Root，而不是等运行时才失败。

涉及：

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`
- `provider-contracts.md` / `provider-contracts.zh-CN.md`

### K4 — Pointer/App Scope 注入时竞态

**问题：** 只在 Batch 前检查 Frontmost App 和坐标不足；Window 或受保护界面可能在 CGEvent 真正投递前发生变化。

**结论：****接受。**

**整改：**

- Checks 1–5 在每一个底层事件发送前重新执行；
- Pointer Event 重新 Hit-test 当前点下的 Window/Application；
- App Scope 与 Protected Surface 改为逐事件检查；
- 任何可观察到的 Scope/Focus/Protected-Surface 变化立即 Abort Batch；
- 最后一次查询到 CGEvent 实际投递之间仍存在 OS 级残余竞态，文档明确承认，不再声称完全消除。

涉及：

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

### K5 — Keyboard Focus Scope

**问题：** `type` / `keypress` 没有坐标，无法依赖 Pointer 的坐标 Scope 防止焦点漂移。

**结论：****接受。**

**整改：**

- 每一个实际发送的 Key Event 前都重新检查 Focused/Frontmost App；
- Focus 必须仍在授权 App Scope 内并且不是 Protected Target；
- Focus 变为越权或受保护目标时立即中止剩余输入。

涉及：

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

### K6 — Phase 0 缺少 Lock/TCC 闭环

**问题：** 两个承重假设没有明确 Spike：

1. daemon 的 Files-and-Folders TCC 身份是否稳定；
2. ScreenCaptureKit / CGEvent 在 screen lock、screen saver、wake、fast user switching 下的真实行为。

**结论：****接受。**

**整改：**

Phase 0 现在显式验证两项。

Computer 在这些状态变化中的安全硬规则：

```text
ComputerSession -> suspended
controller -> none
frame/geometry -> invalid
queued input -> discard
resume -> fresh observation first
```

返回后禁止重放旧的 Agent Input。

涉及：

- `architecture.md` / `architecture.zh-CN.md`
- `roadmap.md` / `roadmap.zh-CN.md`

## 3. P2 保留但不升级

本轮登记但不阻塞 Freeze 的内容：

- 其他拥有 Accessibility 权限的进程产生的 synthetic input；
- Provider-specific orphan process reattach；
- 未来 Run.kind 扩展；
- Session 归档策略；
- CodexBridge `win` 分支可能偏 Windows；
- Widget WebRTC fallback 细节。

在各自触发条件出现之前均保持 Non-blocking。

## 4. 整改后的状态

6 个 P1 都已经有对应的 V2 文档整改。

但本审查本身**不直接授予** `READY_TO_FREEZE`。

下一步应由独立模型对整改后的最新工作树执行最终 Freeze Gate：

- 只报告仍存在的 P0/P1；
- 禁止为了风格/偏好重新设计；
- 如果不存在 P0/P1，则返回 `READY_TO_FREEZE`。

## 5. 审查关闭时仓库状态

本轮整改没有修改任何实现代码。

架构审查相关修改在最终 Freeze Gate 通过前保持未提交状态。
