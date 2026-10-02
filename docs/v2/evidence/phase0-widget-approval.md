# Phase 0 — approval-only widget evidence

## Assumption

ChatGPT can deliver an approval token only in tool-result `_meta`, expose an approval decision tool only to the widget, and round-trip a once/session human decision without WebRTC. UI visibility is not server authentication: possession of a short-lived token is required.

## Environment

2026-10-02; macOS 27.0 arm64; managed Chromium browser. Real ChatGPT Plugins page opened at https://chatgpt.com/plugins and displayed Log in / Sign up. Platform tunnel settings redirected to https://platform.openai.com/login?next=%2Fsettings%2Forganization%2Ftunnels. No authenticated developer-mode conversation available in that session. Runtime API key and tunnel id environment variables absent; no local tunnel profile directory found.

## Commands / steps

- Read primary [Apps SDK reference](https://developers.openai.com/plugins/reference): `_meta.ui.visibility`, tool result `_meta`, host widgetSessionId and resource CSP.
- Opened actual ChatGPT and Platform pages in a browser; observed login requirement and captured screenshots (ephemeral browser artifacts, no account data persisted).
- Implemented opt-in `-widget-spike`: `phase0_approval`, `phase0_approval_decide`, `phase0_signal`, and `ui://codebridge/phase0-approval.html`.
- Opened the actual HTML file in Chromium. Approval controls were disabled without host metadata; no fake `window.openai` bridge was installed.

## Observed result

Actual ChatGPT round trip: **BLOCKED**, not performed. The local HTML denies approval without host metadata. Server regression and UDS smoke results are recorded in the verification/closeout evidence after execution.

The probe requests only fixed `computer.observe`, consumes a token once, expires after five minutes, restricts scope to once/session, has no caller-controlled permission verb or selector, and never writes a Runtime Grant. Tokens are hashed in memory and returned only in result `_meta`; restart loses them. Capacity bounded at 64 unexpired requests. No project/always/secret.read/cloud.upload decision route.

## Conclusion

**FAIL for Phase 0 real-widget acceptance until authenticated ChatGPT + real Tunnel test is executed.** A local browser or MCP test cannot prove OpenAI model-context hiding or UI-only host routing.

## Architecture impact

No amendment. Frozen F4 remains intact; remote approval cannot ship on this evidence alone.

## Follow-up

Use genuine developer-mode access and the user's own workspace-associated tunnel. Invoke `phase0_approval`, inspect that model content has no token, approve/deny using explicit UI, and verify replay/expired/missing-token/wrong-widget rejection.

### Residual risk: widgetSessionId

The primary reference describes `openai/widgetSessionId` as host-provided tool-result metadata, not a guaranteed server request field. Probe binds the token when the server receives it; otherwise token possession is the weaker boundary. Unit/smoke calls with a provided id do not prove ChatGPT supplies it. Both real host cases remain untested. Missing id is a recorded P2 residual risk allowed by the freeze gate, not permission to widen scopes.
