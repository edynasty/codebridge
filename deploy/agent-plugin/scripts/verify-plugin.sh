#!/usr/bin/env bash
# CodeBridge plugin package verifier. Checks the dual-surface plugin layout
# (Agent Plugins portable + Codex compatibility), validates every manifest,
# and exercises the live MCP server end-to-end (OAuth + initialize +
# tools/list).
#
# Usage: bash scripts/verify-plugin.sh [MCP_BASE_URL] [ADMIN_PASSWORD]
set -euo pipefail
cd "$(dirname "$0")/.."

MCP_BASE="${1:-https://cb.edynasty.asia}"
PW="${2:-${CODEBRIDGE_AS_ADMIN_PASSWORD:-}}"
PASS=0; FAIL=0
ok()   { PASS=$((PASS+1)); echo "PASS: $1"; }
bad()  { FAIL=$((FAIL+1)); echo "FAIL: $1"; }

# ── Plugin manifests ───────────────────────────────────────────────────────
for f in plugin.json mcp.json .mcp.json .app.json .codex-plugin/plugin.json; do
  [ -f "$f" ] && python3 -c "import json; json.load(open('$f'))" 2>/dev/null && ok "$f valid JSON" || bad "$f missing or invalid"
done

# Portable: streamable-http. Codex: http.
python3 -c "
import json
t = json.load(open('mcp.json'))['mcpServers']['codebridge']['type']
assert t == 'streamable-http', t" && ok "mcp.json transport = streamable-http (Agent Plugins spec)" || bad "mcp.json wrong transport"
python3 -c "
import json
t = json.load(open('.mcp.json'))['mcpServers']['codebridge']['type']
assert t == 'http', t" && ok ".mcp.json transport = http (Codex spec)" || bad ".mcp.json wrong transport"

# App binding must reference a real connector id (no placeholder).
python3 -c "
import json
d = json.load(open('.app.json'))['apps']['codebridge']
assert d['id'].startswith('connector_') or d['id'].startswith('asdk_app_'), 'placeholder id: ' + d['id']" \
  && ok ".app.json real connector id" || bad ".app.json id is a placeholder (register the connector in ChatGPT first)"

# Codex overlay must bind apps + skills + mcpServers.
python3 -c "
import json
d = json.load(open('.codex-plugin/plugin.json'))
assert d['apps'] == './.app.json'
assert d['skills'] == './skills/'
assert d['mcpServers'] == './.mcp.json'" && ok ".codex-plugin bindings (apps/skills/mcpServers)" || bad ".codex-plugin bindings incomplete"

# Skills carry front-matter.
for s in skills/*/SKILL.md; do
  head -1 "$s" | grep -q '^---$' && grep -q '^name:' "$s" && ok "skill front-matter: $s" || bad "skill front-matter: $s"
done

# Versions consistent at 1.0.1.
for f in plugin.json .codex-plugin/plugin.json; do
  python3 -c "
import json
assert json.load(open('$f'))['version'] == '1.0.1'" && ok "$f version 1.0.1" || bad "$f version mismatch"
done

# ── Live MCP verification ──────────────────────────────────────────────────
if [ -z "$PW" ]; then
  echo "SKIP: live MCP verification (set ADMIN_PASSWORD or CODEBRIDGE_AS_ADMIN_PASSWORD)"
else
  TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
  V=$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")
  C=$(python3 -c "import hashlib,base64; v='$V'.encode(); print(base64.urlsafe_b64encode(hashlib.sha256(v).digest()).rstrip(b'=').decode())")
  CID=$(curl -sf --max-time 20 -X POST "$MCP_BASE/register" -H 'Content-Type: application/json' \
    -d '{"redirect_uris":["http://127.0.0.1:57917/callback"]}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["client_id"])') \
    && ok "DCR registration" || bad "DCR registration"
  R=$(curl -s -o /dev/null -w '%{redirect_url}' --max-time 40 -X POST "$MCP_BASE/authorize" \
    --data-urlencode "client_id=$CID" --data-urlencode "redirect_uri=http://127.0.0.1:57917/callback" \
    --data-urlencode "code_challenge=$C" --data-urlencode "code_challenge_method=S256" \
    --data-urlencode "resource=$MCP_BASE" --data-urlencode "scope=codebridge.read" \
    --data-urlencode "username=admin" --data-urlencode "password=$PW")
  CODE=$(python3 -c "from urllib.parse import urlparse,parse_qs; print(parse_qs(urlparse('$R').query).get('code',[''])[0])")
  [ -n "$CODE" ] && ok "authorization code issued" || bad "authorization code"
  AT=$(curl -sf --max-time 30 -X POST "$MCP_BASE/token" --data-urlencode "grant_type=authorization_code" \
    --data-urlencode "code=$CODE" --data-urlencode "client_id=$CID" \
    --data-urlencode "redirect_uri=http://127.0.0.1:57917/callback" \
    --data-urlencode "code_verifier=$V" | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])') \
    && ok "PKCE token exchange" || bad "PKCE token exchange"

  INIT=$(curl -sf --max-time 30 -X POST "$MCP_BASE/mcp" -H "Authorization: Bearer $AT" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"verify-plugin","version":"1.0.1"}}}')
  echo "$INIT" | grep -q serverInfo && ok "MCP initialize" || bad "MCP initialize"

  TOOLS=$(curl -sf --max-time 30 -X POST "$MCP_BASE/mcp" -H "Authorization: Bearer $AT" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}')
  for want in list_devices list_workspaces read bash permission_grant agent agents_list; do
    echo "$TOOLS" | grep -q "\"$want\"" && ok "tool exposed: $want" || bad "tool missing: $want"
  done
fi

echo
echo "== $PASS passed, $FAIL failed =="
[ "$FAIL" -eq 0 ]
