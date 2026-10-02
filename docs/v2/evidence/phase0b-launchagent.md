# Phase 0B — SMAppService lifecycle / 生命周期

Result: **SIGNED_TCC_BLOCKED_EXTERNAL**. Signed acceptance **NOT RUN**.

## Prepared path / 已准备路径

App → `SMAppService.agent(plistName: "com.codebridge.daemon.plist")` → `Contents/Library/LaunchAgents/com.codebridge.daemon.plist` → `BundleProgram=Contents/MacOS/codebridged`. KeepAlive restarts nonzero exits. The App does not spawn the daemon. `launchctl print` is read-only job inspection, not service registration or acceptance bootstrap.

App 使用 SMAppService 注册 bundled plist，daemon 由 launchd 创建；未使用手工 bootstrap 代替验收。

The new App CLI exposes `--phase0b-service status|register|unregister`. Registration verifies live Apple-anchored App code and matching, separately identified nested daemon/probe before calling SMAppService. `status` requests no Computer grant. The unsigned fixture's status returned `plist not found in the bundle` and no loaded job; the plist existed and passed plutil. This is an observed unsigned SMAppService status, not a signed packaging/lifecycle result. Unsigned register exited 3 before registration.

新增 CLI 的 register 先校验真实 App 与两个 nested executable 身份；status 不请求 Computer 权限。未签名 fixture 的 status 是 `plist not found in the bundle`、job 未加载，即使 plist 文件存在且 plutil 通过；不能把它解释为真实签名的打包/生命周期失败。未签名 register 在注册前 exit 3。

## Required signed acceptance / 待执行真实签名验收

| Step | Required evidence / 必需证据 | Current |
| --- | --- | --- |
| register through App | SMAppService result/status; human Login Items approval if required | UNTESTED |
| launch | daemon executable identity, PID, parent PID=1, bundled plist path | UNTESTED |
| app quit | original daemon PID remains alive and serves IPC | UNTESTED |
| SIGKILL daemon only | launchd replacement PID, parent PID=1, bounded restart latency | UNTESTED |
| Store recovery | persisted schema/state reopened without loss; no fresh-store substitute | UNTESTED |
| IPC recovery | old connection fails cleanly; newly authenticated connection succeeds | UNTESTED |
| App reopen | App reconnects to launchd daemon using live signed peer checks | UNTESTED |
| unregister through App | SMAppService result, disabled/notRegistered status, job removal/process exit | UNTESTED |

Use LaunchServices to launch the actual signed App at the stable installation path, e.g. `open -n -W "$APP" --args --phase0b-service register`; capture stdout where available and query status from the same App. For unregister, allow process reaping before asserting absence; Apple's async completion variant explicitly waits for termination. Do not use whole GUI-domain dumps (may contain credentials). No launchctl bootstrap/bootout counts as acceptance.

通过 LaunchServices 启动真实签名 App；Login Items 要求的人工批准不能由脚本伪造。App quit、kill、Store reopen、IPC recovery、重连、注销每一步都要真实证据。只检查具体 job，不导出包含凭证的整个 GUI 环境。

Primary source: [Apple SMAppService](https://developer.apple.com/tutorials/data/documentation/servicemanagement/smappservice.md); installed SDK SMAppService.h. Updates to an executable/plist require re-registration; App-parent versus responsible-process identity still requires separate [TCC evidence](phase0b-tcc.md).
