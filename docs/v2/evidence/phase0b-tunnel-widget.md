# Phase 0B — Actual ChatGPT / Tunnel / approval widget

Result: **BLOCKED_EXTERNAL_CHATGPT_CREDENTIALS**. Actual ChatGPT → Secure MCP Tunnel → codebridged acceptance **NOT RUN**.

## Fresh prerequisite checks / 本轮前提检查

- Presence-only check: CODEBRIDGE_TUNNEL_ID, CONTROL_PLANE_TUNNEL_ID, CONTROL_PLANE_API_KEY, OPENAI_TUNNEL_ID, OPENAI_TUNNEL_API_KEY and OPENAI_API_KEY unset; no environment variable names matching Tunnel/control-plane/ChatGPT configuration. No secret values printed. Known user tunnel-profile globs found no matching profile.
- Managed Chromium `https://chatgpt.com/plugins`: actual Log in / Sign up surface, no authenticated developer-mode conversation. Platform tunnels URL redirected to `https://platform.openai.com/login?next=%2Fsettings%2Forganization%2Ftunnels`, displaying the actual login form. Managed tabs were closed.
- Read-only actual-browser relay attempt targeted existing chatgpt.com without navigation: relay service was present, but its extension never connected. This does not establish the user's real browser login state; it means that surface was inaccessible to this session. No extension install or browser/account setting change was made.
- Credential absence/inaccessible authenticated surface is an **external prerequisite**, not a tunnel transport architecture failure.

仅检查凭证是否存在，不输出值。受管浏览器显示 ChatGPT 登录入口，Platform Tunnel 设置重定向到真实登录表单。读取实际用户浏览器的 relay 因 extension 未连接而不可用；不据此声称用户所有浏览器均未登录。未安装 extension 或改动账户/浏览器配置。此为外部前提缺失，不是 transport 架构失败。

## Preserved local results / 保留本地结果

Previous actual local UDS MCP/supervised tunnel argv smoke, native host calls and approval token/scope/replay enforcement remain **local PASS evidence only**: [Tunnel](phase0-tunnel.md), [IPC](phase0-ipc.md), [widget](phase0-widget-approval.md). No ingress/approval policy was changed. No old probe_grant_free/local HTML simulation is promoted to actual ChatGPT acceptance, and no real E2E PASS is claimed.

历史本地 UDS/MCP 与 token/scope/replay PASS 不撤销，但不升级为真实 ChatGPT E2E。未修改 ingress/approval 策略，也不把 grant-free probe 或本地 HTML 模拟当作真实 widget 验收。

## Minimum next acceptance / 最小下一步验收

1. Provide accessible authenticated ChatGPT developer-mode conversation and the account's valid tunnel ID/control-plane credentials; place secrets only in protected runtime configuration, never evidence or commits. Confirm correct tunnel target is the daemon's authenticated UDS Streamable HTTP MCP endpoint.
2. Run daemon-supervised real tunnel-client; capture bounded lifecycle/status and transport evidence without credentials. ChatGPT calls a real native daemon host command (no Manager/container intermediary), preserving tool operation/correlation identity.
3. A policy-ask operation creates a pending approval **without auto-approval**. Actual ChatGPT widget receives token in tool-result `_meta`, a human uses the UI-only path, and that grant resumes/executes **the original operation**, not a new grant-free probe. Record ask → token → UI decision → original operation result.
4. Verify once/session scopes, expiry, replay, wrong approval/operation identity, and model/tool-list inability to mint or widen grants on the real supported surface. Revoke/deny behaves correctly. WebRTC/media is Phase 2 and not added here.

需要可访问的真实 developer-mode 会话和有效 Tunnel/control-plane 配置；secret 不进入证据或 commit。真实 widget 的人工审批必须恢复原始 policy-ask 操作，并实测 token/scope/replay 边界；不允许自动批准，不涉及 WebRTC。
