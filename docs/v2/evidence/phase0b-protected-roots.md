# Phase 0B — Protected roots / 保护目录

Result: **SIGNED_TCC_BLOCKED_EXTERNAL**. V1 protected-root outcome: **UNDETERMINED**, not SUPPORTED and not UNSUPPORTED_IN_V1. Missing signing identity is an external prerequisite, not a root-permission architecture failure.

结论：缺真实签名；v1 保护目录支持结果仍未确定，不能因缺证书宣布不支持。

## Preparation and actual smoke / 准备与实际 smoke

- Existing root probe retains a five-second **report deadline per root** around native filesystem access. A timed-out detached thread is not canceled; it may remain blocked until this short-lived process exits. This is not a production ProjectRegistration implementation or proof that kernel I/O was interrupted. `blocked_seconds` means unresolved access/existence; do not infer DENIED or use legacy exists=true as an observed stat result in that case.
- New `files-folders` mode exercises root access independently, without ScreenCaptureKit, AX or event-tap calls. It adds optional `files_folders` to the existing report; permissions reports keep their existing shape. App same-process version remains genuinely signed-gated.
- Real standalone probe smoke used **owned ordinary temporary roots only**: a directory with two entries was readable/count=2; a missing path returned exists=false/readable=false; a regular file returned exists=true/readable=false/errno=20 (ENOTDIR). Actual report duration 0.017525s. No protected root, grant request or permission-persistence experiment ran. This ordinary-root smoke is not signed acceptance.
- 保留每个 root 五秒的报告 deadline；超时线程不是被取消的 native I/O，不等于生产 ProjectRegistration。超时的 existence/access 未知，不能据此认定拒绝。新增独立 files-folders 模式；只对自有普通临时目录验证了读取、缺失路径、非目录边界，未运行保护目录或授权持久性实验。

## Required signed acceptance / 待执行真实签名验收

| Root | Before grant | Explicit human Files & Folders grant/result | Daemon restart | Same-identity rebuild/update | Decision |
| --- | --- | --- | --- | --- | --- |
| ~/Documents | UNTESTED | UNTESTED | UNTESTED | UNTESTED | UNDETERMINED |
| ~/Desktop | UNTESTED | UNTESTED | UNTESTED | UNTESTED | UNDETERMINED |
| ~/Downloads | UNTESTED | UNTESTED | UNTESTED | UNTESTED | UNDETERMINED |
| Ordinary owned root | unsigned probe-only boundaries PASS | no grant involved | UNTESTED signed path | UNTESTED signed path | not protected-root evidence |

Run from the genuine SMAppService-owned daemon identity, and separately its signed harness; do not substitute shell access or give daemon Computer grants. Record PID/identity, exact root (no child names), grant recipient, request attribution, errno/deadline, restart and same-identity update results. Use a bounded child/process deadline as well as per-root report deadline, and kill only an owned probe if native I/O stalls; never wait forever for a TCC dialog. The previous blocked-open finding remains in [Phase 0 TCC evidence](phase0-tcc.md).

For supported roots, document observed denial/error behavior and `permission_denied: host_permission_required` guidance: the user grants Files & Folders access to the actually attributed component in System Settings and retries the original registration; do not request Full Disk Access or grant daemon Screen Recording/AX/Input Monitoring to bypass it. A timeout is an unresolved host-permission probe, not a proven denial. If signed evidence establishes an unsupported protected-root case, choose UNSUPPORTED_IN_V1 with that evidence and an ordinary-root alternative. No production registration API was added here.

以真实 SMAppService daemon 身份和独立 harness 分别验收，记录授权对象、错误/deadline、重启及相同身份更新结果。不能用 shell 权限代替，也不授予 daemon Computer 权限。支持时提供 host_permission_required 指引与重试原注册操作；不通过 Full Disk Access 绕过。只有真实签名证据支持时才选择 UNSUPPORTED_IN_V1。
