# Phase 0 — widget WebRTC feasibility evidence

## Assumption

RTCPeerConnection and UI-only MCP signaling work inside real ChatGPT widget sandboxes on Web, Desktop, Mobile; CSP and ICE support STUN/TURN as needed.

## Environment

2026-10-02; macOS 27.0 arm64; actual local widget HTML in managed Chromium. ChatGPT.app exists in /Applications. No authenticated developer-mode ChatGPT widget or mobile device/test session was available through the exercised tools.

## Commands / steps

Opened `file:///Users/tangxingpeng/IdeaProjects/me/codebridge/internal/approvalspike/widget.html`, observed actual controls, clicked Test widget WebRTC, waited for data-channel message, captured screenshot, closed tab.

## Observed result

Local two-peer RTCPeerConnection offer/answer and data-channel payload succeeded:

```json
{
  "rtc": "local data channel passed",
  "signaling": "unavailable: not a mounted approval widget",
  "host": "not ChatGPT widget",
  "stun": "not tested",
  "turn": "not tested",
  "csp": "actual host CSP must be checked on each ChatGPT surface"
}
```

The served resource declares `_meta.ui.csp.connectDomains=[]` and `resourceDomains=[]`; there are no external requests in the local experiment. `phase0_signal` requires the widget-only token and bounds SDP to 64 KiB. It is an SDP transport probe, not a media service or ComputerSession.

| Surface / check | Result |
| --- | --- |
| Local Chromium data channel | PASS (local only) |
| ChatGPT Web widget | NOT TESTED — authenticated developer-mode widget unavailable |
| ChatGPT Desktop widget | NOT TESTED — no configured tunnel/widget session |
| ChatGPT Mobile widget | NOT TESTED — no actual device/session |
| ChatGPT CSP / connectDomains enforcement | NOT TESTED |
| Real UI-only signaling | NOT TESTED |
| STUN | NOT TESTED |
| TURN | NOT TESTED |

## Conclusion

**DEFERRED for Phase 2 only**, as explicitly gated by the frozen roadmap. This is not a ChatGPT WebRTC PASS and does not waive Phase 1 approval-only acceptance. No complete Live Computer or media transport was implemented.

## Architecture impact

No evidence that widget WebRTC is impossible. Conditional frozen fallback remains **stateless rendezvous relay**, only if real widget/signaling infeasibility is demonstrated. No Manager is introduced.

## Follow-up

Run the actual widget on each supported ChatGPT surface; record host/app versions and enforced CSP. Test configured STUN and TURN on restrictive networks with real signaling. If unavailable, choose the frozen stateless rendezvous fallback before Phase 2.
