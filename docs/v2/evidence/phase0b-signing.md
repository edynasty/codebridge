# Phase 0B — Signing / 签名

Date: 2026-10-02. Result: **SIGNED_TCC_BLOCKED_EXTERNAL**.

## Observed / 实测

- macOS 27.0 (26A428), arm64, GUI uid 501. Actual `security find-identity -v -p codesigning`: **0 valid identities found**. No certificate or private key was installed by this pass.
- Real builder invocation exited 2 with the external-blocker classification before Swift compilation or bundle replacement; an owned existing-output marker remained unchanged. See [raw smoke](phase0b-smoke.json).
- Current signed tree: **NOT BUILT / NOT VERIFIED**. No actual Authority, TeamIdentifier or designated requirement for an Apple-signed candidate can be reported. No fake Team ID or ad-hoc signing is acceptance evidence.
- 本轮未安装证书或私钥。真实构建入口在编译或替换 App 前以 exit 2 拒绝，原输出标记保持不变；真实 Apple 签名树尚未生成/验证，不填造 Authority、TeamIdentifier 或 designated requirement。

## Prepared identity contract / 已准备的身份契约

| Component / 组件 | Required signing identifier | Actual Apple-signed metadata / 实际真实签名元数据 |
| --- | --- | --- |
| App | com.codebridge.app | UNTESTED — certificate missing |
| daemon | com.codebridge.daemon | UNTESTED — certificate missing |
| probe | com.codebridge.probe | UNTESTED — certificate missing |

Apple Development is preferred for development; Developer ID Application is also accepted. A shared **genuine** Team is allowed; identifiers must differ. The builder selects an available Apple Development identity first when none is specified, signs an owned temporary executable, checks Apple trust, and derives Team from actual metadata rather than a certificate display-name suffix. An optional configured Team must match. Nested probe/daemon are signed first, App last, with hardened runtime.

开发优先 Apple Development，也接受 Developer ID Application；允许同一个真实 Team，但三个 identifier 必须不同。构建入口先用自有临时 executable 验证私钥和 Apple 信任链，再从真实签名元数据提取 Team；配置 Team 仅作为必须匹配的期望值。正向真实证书路径本轮 **UNTESTED**。

The App's sealed Info.plist carries expected daemon identifier/Team. The bundled LaunchAgent carries the App identifier/Team needed by daemon authorization. This does not relax live audit-token/PID, UID, Apple-anchor or role=app checks. Unsigned development bundles do not supply a trusted Team and cannot register through the new acceptance entry point.

App 的签名封存 Info.plist 携带期望 daemon 身份，LaunchAgent 携带 daemon 验证 App 所需的身份。未放宽 live peer、UID、Apple anchor 或 app role 检查。未签名开发构建不提供可信 Team，新验收入口拒绝注册和 OS 权限探针。

## Next real build / 下一次真实构建

After installing a certificate **with its private key** and verifying it appears in security's valid identity list:

```sh
make build-daemon
desktop/macos/Building/build-app.sh --daemon "$PWD/bin/codebridged" --embed-phase0-env
```

Keep the resulting App at a stable path. Stop/unregister an existing service before replacing its executable/plist, then register through the rebuilt App. No notarization requirement is inferred for the development LaunchAgent from Apple's separate LaunchDaemon notarization requirement.

For **each** App/daemon/probe, the signed builder emits full `codesign -dv --verbose=4`, `codesign -d -r-`, and `codesign --verify --deep --strict --verbose=4` with an Apple-anchored identifier/Team requirement. Preserve full output: Authority chain, TeamIdentifier, Identifier, CodeDirectory flags/runtime and designated requirement. Rebuild/re-sign using the same real Team and identifiers before persistence repeats.

安装含私钥的真实证书并确认 identity 有效后再运行上述命令；保持稳定 App 路径。替换已有服务文件前先停止/注销，再由新 App 注册。逐组件保留完整 codesign 元数据、严格验证与 designated requirement；本轮只验证了无证书拒绝路径，不能宣称正向签名验收通过。
