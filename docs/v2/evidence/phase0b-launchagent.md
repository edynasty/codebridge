# Phase 0B — SMAppService lifecycle / 生命周期

Date: 2026-10-03. Result: **SMAppService signed lifecycle PASS**. This is not an F2 isolation PASS.

## 2026-10-03 signed lifecycle / 本轮真实生命周期

**OBSERVED** — All registrations/unregistrations used the actual signed App via LaunchServices and `SMAppService.agent(plistName:)`. `launchctl print gui/501/com.codebridge.daemon` was inspection only. No manual bootstrap/submit/bootout substituted for SMAppService.

**OBSERVED / fix** — Initial signed registration returned enabled, but the daemon exited 2 and launchd scheduled another spawn. The bundled plist supplied no `run` argument; codebridged requires a subcommand. Unregistered through App, added `ProgramArguments=[codebridged, run]`, rebuilt the signed bundle and re-registered. Retained the initial failure in [raw evidence](phase0b-signed-2026-10-03.json).

| Acceptance step | OBSERVED result |
| --- | --- |
| Register | App result enabled; launchd job managed by com.apple.xpc.ServiceManagement; signed daemon PID 35517, parent 1 |
| App quit → daemon survives | actual normal menu-bar App opened and quit; PID 35517 remained alive and host.health succeeded |
| Kill → restart | SIGKILL only PID 35517; replacement PID 47519, parent 1, observed running by the 12s inspection |
| Store reopens | same `~/Library/Application Support/CodeBridge/runtime.db`, inode 366202558, schema 1, wired=true before/after; no new-store substitute |
| IPC reconnects | held connection returned EOF after kill; a new diagnostics connection succeeded; signed App role=app reconnected to PID 47519 with audit token, verified daemon signature and allowUnverifiedPeer=false |
| Same-identity update | App unregister removed the job; same Team/identifiers rebuilt and strictly verified; App re-register started PID 50184 |
| Unregister | actual App result not registered, job inspection exit 113, daemon PID 50184 absent |
| Final source/native report smoke | rebuilt final source; App register started PID 60728; daemon-in-process Files & Folders report matched that live PID/identity; App unregister removed the job again |
| Actual App surface | accessibility hierarchy exposed menu-bar item CB and the real handshake/register/unregister/probe/Quit menu entries |

**OBSERVED limits** — The reopened Store contained no Runs/Sessions/journal entries before this crash experiment; the persisted schema/database reopening is proven, not populated-domain crash recovery. `open -W` sometimes exited 1 with `initial call to kevent() failed: No such process` for short-lived CLI App instances; the actual JSON reports still showed successful signed handshakes/Files & Folders calls. Those wrapper errors are retained, not converted to an App error or silently rerun.

**DECISION** — Leave the final service **unregistered** and App quit, as at scope. Preserve the signed App at its stable path for the next human TCC step. No durable user data, bearer token or unrelated GUI environment was exported. Targeted launchctl captures omit inherited GUI environment.

**BLOCKED** — [F2](phase0b-tcc.md), not SMAppService, prevents Phase 1 readiness. Launchd ownership and separate signing identifiers do not prove independent TCC subjects.

The remaining sections preserve the **2026-10-02 preparation/unsigned observations**; their UNTESTED table is historical, superseded by the signed results above.


## Prepared path / 已准备路径

App → `SMAppService.agent(plistName: "com.codebridge.daemon.plist")` → `Contents/Library/LaunchAgents/com.codebridge.daemon.plist` → `BundleProgram=Contents/MacOS/codebridged`. KeepAlive restarts nonzero exits. The App does not spawn the daemon. `launchctl print` is read-only job inspection, not service registration or acceptance bootstrap.

App 使用 SMAppService 注册 bundled plist，daemon 由 launchd 创建；未使用手工 bootstrap 代替验收。

The new App CLI exposes `--phase0b-service status|register|unregister`. Registration verifies live Apple-anchored App code and matching, separately identified nested daemon/probe before calling SMAppService. `status` requests no Computer grant. The unsigned fixture's status returned `plist not found in the bundle` and no loaded job; the plist existed and passed plutil. This is an observed unsigned SMAppService status, not a signed packaging/lifecycle result. Unsigned register exited 3 before registration.

新增 CLI 的 register 先校验真实 App 与两个 nested executable 身份；status 不请求 Computer 权限。未签名 fixture 的 status 是 `plist not found in the bundle`、job 未加载，即使 plist 文件存在且 plutil 通过；不能把它解释为真实签名的打包/生命周期失败。未签名 register 在注册前 exit 3。

## Historical 2026-10-02 planned signed acceptance / 历史计划

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
