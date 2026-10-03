# Phase 0B — Signing / 签名

Date: 2026-10-03. Result: **IDENTITY VALID; SIGNING SMOKE PASS; SIGNED TREE VERIFIED**. F2 remains **NOT CLOSED**.

## 2026-10-03 trust-chain repair / 本轮证书链修复

**OBSERVED** — Initial matching identity was `B1DB75FAC36735E9BE6B1D0FA7A96BB60FD79649`, with zero valid identities. The target certificate already had a matching private key. The only WWDR candidate in the search list was the legacy certificate expiring 2023-02-07; no OU=G3 intermediate was present. Apple Root CA was present in `/System/Library/Keychains/SystemRootCertificates.keychain`. No Apple Development/WWDR/Apple Root trust override appeared in either user or administrator trust settings. Login was already the default user keychain and in the search list.

| Target certificate field | OBSERVED |
| --- | --- |
| subject | UID=76VS5K9978; CN=Apple Development: tangxingpeng@hotmail.com (QC2AU8UDY6); OU=HXAV5ALQQG; O=星鹏 汤; C=US |
| issuer | CN=Apple Worldwide Developer Relations Certification Authority; OU=G3; O=Apple Inc.; C=US |
| notBefore | 2026-10-03 00:01:01 UTC |
| notAfter | 2027-10-03 00:01:00 UTC |
| SHA256 | F4:8D:DB:09:88:88:88:9F:59:F0:53:A9:26:2A:EB:E8:0C:99:77:7E:EE:69:2B:70:20:F6:8C:7C:7C:5F:69:2F |

**OBSERVED** — System time at diagnosis was 2026-10-03 08:19:13 CST, within this validity interval. No time policy was changed. Downloaded [official Apple WWDR G3](https://www.apple.com/certificateauthority/AppleWWDRCAG3.cer), linked by [Apple PKI](https://www.apple.com/certificateauthority/) and [Apple Developer's generation mapping](https://developer.apple.com/help/account/certificates/wwdr-intermediate-certificates). Verified the intermediate with the built-in trust store before importing **one certificate into login.keychain-db**. G3 SHA256: `DC:F2:18:78:C7:7F:41:98:E4:B4:61:4F:03:D6:96:D8:9C:66:C6:60:08:D4:24:4E:1B:99:16:1A:AC:91:60:1F`; validity ends 2030-02-20. G3 is the certificate's OU, not a suffix in its CN; the literal name query containing “Authority G3” alone is insufficient to identify installed generations.

**OBSERVED** — Import returned `1 certificate imported`; target code-signing certificate verification succeeded; `security find-identity -v -p codesigning` then returned the exact target and **1 valid identities found**. No certificate/private key was deleted or regenerated; no Root CA, trust setting, Always Trust, search list or TCC database was changed. Initial curl calls returned 403; the official download succeeded with `curl -q --noproxy '*'`. No curl/network configuration was edited.

**DECISION** — The requested display name's suffix `QC2AU8UDY6` is not the signed Team. Both certificate OU and the actual signed executable report **HXAV5ALQQG**. The user explicitly selected **使用实测 HXAV5ALQQG**. All subsequent builds/checks use that genuine Team; the original QC2AU8UDY6 Team expectation is not claimed satisfied.

**OBSERVED** — Compiled the requested minimal C binary with clang, signed with the exact identity, strictly verified and executed it successfully. Authority was target Apple Development → WWDR → Apple Root CA; TeamIdentifier=HXAV5ALQQG; signature was not ad-hoc. Removed the smoke source/binary. **PASS under the user-approved actual-Team contract**.

## 2026-10-03 actual signed tree / 真实签名树

Stable bundle: `desktop/macos/build/CodeBridge.app`. Final source was rebuilt/signed after the Go/Swift checks. All three components passed metadata display, designated-requirement display, strict verification and explicit Apple-anchor/identifier/Team verification; App also passed deep verification. All carry hardened runtime and TeamIdentifier=HXAV5ALQQG.

| Component | OBSERVED Identifier | OBSERVED Authority chain |
| --- | --- | --- |
| CodeBridge.app | com.codebridge.app | Apple Development: tangxingpeng@hotmail.com (QC2AU8UDY6) → WWDR → Apple Root CA |
| bundled codebridged | com.codebridge.daemon | same genuine chain |
| bundled codebridge-probe | com.codebridge.probe | same genuine chain |

**OBSERVED Designated Requirements** — For each identifier above, exact output is `identifier "<identifier>" and anchor apple generic and certificate leaf[subject.CN] = "Apple Development: tangxingpeng@hotmail.com (QC2AU8UDY6)" and certificate 1[field.1.2.840.113635.100.6.2.1] /* exists */`. Full per-component output, CDHashes and signatures are preserved in [raw signed run](phase0b-signed-2026-10-03.json).

**OBSERVED / fix** — The first genuine builder run failed: `anchor apple generic: No such file or directory; invalid requirement specification`. Both inline `--test-requirement` arguments lacked their required `=` prefix. Corrected only those arguments; the same genuine build then passed. No verification was suppressed.

**BLOCKED** — Signing is no longer the blocker. [F2](phase0b-tcc.md) still lacks the App's real Computer ALLOWED condition and uncontaminated independent-subject evidence. No architecture cutover or private responsibility API was added.

The remaining sections below preserve the **2026-10-02 historical preparation**, not the current signing result.


## Historical 2026-10-02 observations / 历史实测

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
