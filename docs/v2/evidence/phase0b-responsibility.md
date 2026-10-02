# Phase 0B — Public responsibility contingency investigation

Date: 2026-10-02. Verdict: **CONTINGENCY_NOT_PUBLICLY_SUPPORTED**. No implementation or SPI linkage. F2 remains frozen and NOT CLOSED. [中文](phase0b-responsibility.zh-CN.md).

## Evidence and limits

1. [Apple SMAppService](https://developer.apple.com/tutorials/data/documentation/servicemanagement/smappservice.md) documents public register/unregister/status and bundled LaunchAgent registration. Installed SDK `ServiceManagement/Headers/SMAppService.h` requires code signing; it describes BundleProgram, immediate per-user bootstrap and update/re-registration. It does not document a responsibility disclaimer. Its notarization note concerns **LaunchDaemons**, not a blanket development LaunchAgent requirement.
2. [Apple launch environment/library constraints](https://developer.apple.com/tutorials/data/documentation/security/applying-launch-environment-and-library-constraints.md) explicitly separates self, parent and responsible processes. A direct helper can have App as parent and responsible process; an XPC service can have launchd as parent while App remains responsible. Constraints **validate** these relationships. They do not document an API to drop responsibility or remove inherited TCC authorization. Therefore parent PID 1 alone is insufficient.
3. Installed public SDK: `/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk`. Read `usr/include/spawn.h` and `usr/include/sys/spawn.h`; a targeted search of public `usr/include` found no `responsibility_spawnattrs_setdisclaim`, `posix_spawnattr_setdisclaim`, `responsibility_set` or `responsibility_get` declaration. Public posix_spawn flags do not establish a disclaimer API.
4. [Chromium launch_mac.cc](https://chromium.googlesource.com/chromium/src/base/+/master/process/launch_mac.cc) (observed blob `7e09a9d4a676c7a91b3a68ef6e173c590c384b5a`) externally declares `responsibility_spawnattrs_setdisclaim` and invokes it for `disclaim_responsibility`. Open-source use is not Apple public API documentation or stability support. No such call was added to CodeBridge.
5. Targeted Apple documentation/forum searches found no documented stable disclaimer setter. An attempted Apple forum thread read encountered human verification; its contents are **not evidence** here. No assumption is made from search snippets alone.

This is a bounded conclusion about an available **supported public implementation**, not proof that an internal symbol cannot exist or that independent launchd identity cannot isolate TCC. No private API, SPI, undocumented entitlement, TCC database write or responsibility shim is acceptable for this gate.

## Decision / conditional minimal amendment

Use distinct genuine signing identifiers with a real Team, actual SMAppService launch and controlled TCC attribution first. Certificate absence does not justify an architecture amendment. There was no signed isolation experiment in this pass, so **no amendment is applied**.

**Only if** controlled, uncontaminated signed experiments reproducibly show App Computer grants authorizing daemon/harness calls: retain F2's security outcome, remove the unsupported disclaimer as a guaranteed implementation escape hatch, and require a documented public launch/identity boundary with repeated denial evidence before any computer.input ships. If no public boundary can satisfy that outcome, remain blocked and return the concrete signed evidence for an explicit architecture decision. Do not silently introduce another privileged helper/topology or lower peer authentication.

The existing architecture text is preserved with this evidence qualification. No private contingency was implemented.
