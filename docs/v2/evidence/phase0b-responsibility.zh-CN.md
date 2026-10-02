# Phase 0B — 公开 responsibility 备用方案调研

日期：2026-10-02。结论：**CONTINGENCY_NOT_PUBLICLY_SUPPORTED**。未实现、未链接 SPI。F2 保持冻结且 NOT CLOSED。[English](phase0b-responsibility.md)。

## 证据与范围

1. [Apple SMAppService](https://developer.apple.com/tutorials/data/documentation/servicemanagement/smappservice.md) 公开 register/unregister/status 与 bundled LaunchAgent 注册。已读本机 SDK `ServiceManagement/Headers/SMAppService.h`：需要代码签名，说明 BundleProgram、按用户 bootstrap 及更新后重新注册；没有公开 responsibility disclaimer。其 notarization 要求针对 **LaunchDaemons**，不能扩展为开发 LaunchAgent 的全面要求。
2. [Apple launch environment/library constraints](https://developer.apple.com/tutorials/data/documentation/security/applying-launch-environment-and-library-constraints.md) 区分 self、parent 和 responsible process。直接 helper 可以同时以 App 为 parent/responsible；XPC 的 parent 可以是 launchd，而 responsible 仍是 App。Constraints 是**关系校验**，不是放弃 responsibility 或移除 TCC 归属的 setter。因此 ppid=1 不足以证明隔离。
3. 已读本机公开 SDK `/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk` 中的 `usr/include/spawn.h`、`usr/include/sys/spawn.h`。对公开 usr/include 的定向检索未找到 `responsibility_spawnattrs_setdisclaim`、`posix_spawnattr_setdisclaim`、`responsibility_set` 或 `responsibility_get` 声明；公开 posix_spawn flags 不能建立 disclaimer API。
4. [Chromium launch_mac.cc](https://chromium.googlesource.com/chromium/src/base/+/master/process/launch_mac.cc)（实际观察 blob `7e09a9d4a676c7a91b3a68ef6e173c590c384b5a`）自行 extern 声明 `responsibility_spawnattrs_setdisclaim` 并在 disclaim_responsibility 时调用。开源项目使用不等于 Apple 公开 API 文档或稳定性支持。CodeBridge 未加入此调用。
5. 针对 Apple 文档/论坛的检索未找到有文档的稳定 disclaimer setter。一次 Apple forum 正文读取被 human verification 阻断，正文**不作为本轮证据**；不依据搜索摘要断言。

结论限定于目前可建立的**受支持公开实现**，不证明内部符号不存在，也不证明独立 launchd 身份无法隔离 TCC。本门槛不接受 private API、SPI、未公开 entitlement、TCC 数据库写入或 responsibility shim。

## 决策与条件性最小修订

先使用不同的真实 signing identifier、真实 Team、实际 SMAppService 启动与受控 TCC 归属实验。缺少证书不能构成架构修订理由；本轮没有签名隔离实验，因此**未应用架构修订**。

**仅当**无历史污染的受控真实签名实验可复现地证明 App Computer grant 授权 daemon/harness：保留 F2 的安全目标，移除把无公开支持的 disclaimer 当作必然可用逃生通道的实现承诺；在提供任何 computer.input 前，要求有文档的公开启动/身份边界及重复拒绝证据。如果没有公开边界满足目标，继续阻塞，携带具体签名实验证据提交显式架构决策，不暗中引入新特权 helper/拓扑，不降低 peer authentication。

原架构文字保留，并加上这项证据限定；没有实现私有备用方案。
