#!/usr/bin/env bash
# Bring up the local CodeBridge preview stack (Manager + Client) and enroll
# the client automatically. Idempotent: safe to re-run.
#
# Usage: bash deploy/local-preview.sh [down|status|logs]
set -euo pipefail

cd "$(dirname "$0")"

ENV_FILE=".env"
MANAGER_PORT="${CODEBRIDGE_MANAGER_PORT:-8180}"
UI_PORT="${CODEBRIDGE_UI_PORT:-8190}"
STATE_DIR="client-state"
PROJECTS_DIR="${HOST_PROJECTS_DIR:-$HOME/IdeaProjects}"

# Real host identity so the device registers as this machine.
export HOSTNAME_SHORT
export HOSTNAME_SLUG
export HOST_USER
HOSTNAME_SHORT=$(hostname -s)
HOSTNAME_SLUG=$(printf '%s' "$HOSTNAME_SHORT" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9-_' '-' | sed 's/^-\+\|-\+$//g')
HOST_USER=$(id -un)

random_token() { openssl rand -hex 24; }

seed_client_config() {
  if [ ! -f "${STATE_DIR}/client.json" ]; then
    mkdir -p "${STATE_DIR}"
    {
      printf '{\n  "manager_url": "ws://manager:8080/agent",\n  "allow_insecure_ws": true,\n  "workspaces": {\n'
      first=1
      for dir in "$PROJECTS_DIR"/*/; do
        [ -d "$dir" ] || continue
        name=$(basename "$dir")
        # container workspaces are mounted under /workspaces/<name>
        if [ $first -eq 1 ]; then first=0; else printf ',\n'; fi
        printf '    "%s": "/workspaces/%s"' "$name" "$name"
      done
      printf '\n  }\n}\n'
    } > "${STATE_DIR}/client.json"
    echo "seeded ${STATE_DIR}/client.json with projects from ${PROJECTS_DIR} (editable at runtime via the UI)"
  fi
}

sync_host_env() {
  # Persist host identity into .env so every later `docker compose` call
  # (not just this script) sees the same device identity.
  set_kv() {
    if grep -q "^$1=" "$ENV_FILE"; then
      sed -i '' "s|^$1=.*|$1=$2|" "$ENV_FILE"
    else
      echo "$1=$2" >> "$ENV_FILE"
    fi
  }
  set_kv HOSTNAME_SHORT "$HOSTNAME_SHORT"
  set_kv HOSTNAME_SLUG "$HOSTNAME_SLUG"
  set_kv HOST_USER "$HOST_USER"
  set_kv HOST_PROJECTS_DIR "$PROJECTS_DIR"
}

ensure_env() {
  if [ ! -f "$ENV_FILE" ]; then
    cat > "$ENV_FILE" <<EOF
# Local preview environment (gitignored). Regenerate freely.
CODEBRIDGE_ADMIN_TOKEN=$(random_token)
CODEBRIDGE_MCP_TOKEN=$(random_token)
CODEBRIDGE_DEVICE_ID=codebridge-local
CODEBRIDGE_DEVICE_NAME=CodeBridge Docker Local
# Stop auto-removing the enroll code after first use if you plan to recreate
# the client-state directory.
CODEBRIDGE_ENROLL_CODE=
EOF
    chmod 600 "$ENV_FILE"
    echo "created $ENV_FILE with fresh tokens"
  fi
}

manager_admin() { # method path [json-body]
  set +e
  if [ $# -ge 3 ]; then
    out=$(curl -sS -X "$1" "http://127.0.0.1:${MANAGER_PORT}$2" \
      -H "Authorization: Bearer $(grep '^CODEBRIDGE_ADMIN_TOKEN=' "$ENV_FILE" | cut -d= -f2-)" \
      -H 'Content-Type: application/json' -d "$3" 2>&1)
  else
    out=$(curl -sS -X "$1" "http://127.0.0.1:${MANAGER_PORT}$2" \
      -H "Authorization: Bearer $(grep '^CODEBRIDGE_ADMIN_TOKEN=' "$ENV_FILE" | cut -d= -f2-)" 2>&1)
  fi
  rc=$?
  set -e
  echo "$out"
  return $rc
}

wait_manager() {
  for _ in $(seq 1 60); do
    if curl -sf "http://127.0.0.1:${MANAGER_PORT}/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "manager did not become healthy on port ${MANAGER_PORT}" >&2
  return 1
}

set_enroll_code() {
  local code="$1"
  if grep -q '^CODEBRIDGE_ENROLL_CODE=' "$ENV_FILE"; then
    sed -i '' "s|^CODEBRIDGE_ENROLL_CODE=.*|CODEBRIDGE_ENROLL_CODE=${code}|" "$ENV_FILE"
  else
    echo "CODEBRIDGE_ENROLL_CODE=${code}" >> "$ENV_FILE"
  fi
}

case "${1:-up}" in
  down)
    docker compose -f compose.client.yml down
    exit 0
    ;;
  status)
    docker compose -f compose.client.yml ps
    exit 0
    ;;
  logs)
    docker compose -f compose.client.yml logs --tail 50 "${2:-}"
    exit 0
    ;;
esac

ensure_env
sync_host_env
seed_client_config

# 1. Manager first; the client retries, but enroll codes must exist before it
#    can register, so the manager must be up and seeded first.
docker compose -f compose.client.yml up -d --build manager
wait_manager

# 2. If the client has no stored credential, mint an enrollment code so it can
#    self-enroll on first connect.
if [ ! -f "${STATE_DIR}/credentials.json" ]; then
  echo "no stored client credential; creating one-time enrollment code"
  resp=$(manager_admin POST /admin/enrollments '{"ttl_seconds":600,"account_id":"default"}')
  code=$(printf '%s' "$resp" | sed -n 's/.*"enrollment_code":"\([^"]*\)".*/\1/p')
  if [ -z "$code" ]; then
    echo "failed to create enrollment code: $resp" >&2
    exit 1
  fi
  set_enroll_code "$code"
  echo "enrollment code stored in $ENV_FILE (single-use, expires in 10 minutes)"
else
  echo "client credential already present; skipping enrollment"
  set_enroll_code ""
fi

# 3. Client.
docker compose -f compose.client.yml up -d --build client

# 4. Verify registration.
sleep 2
for _ in $(seq 1 20); do
  devices=$(manager_admin GET /admin/devices || true)
  if printf '%s' "$devices" | grep -q '"online":true'; then
    echo
    echo "=== client is online ==="
    printf '%s\n' "$devices"
    echo
    echo "manager : http://127.0.0.1:${MANAGER_PORT} (admin token in deploy/.env)"
    echo "MCP     : http://127.0.0.1:${MANAGER_PORT}/mcp (bearer: CODEBRIDGE_MCP_TOKEN in deploy/.env)"
    echo "client UI: http://127.0.0.1:${CODEBRIDGE_UI_PORT:-8190} (edit workspaces, connection and local policy; saves hot-reload)"
    echo "control : bash deploy/local-preview.sh [status|logs|down]"
    exit 0
  fi
  sleep 1
done

echo "client did not come online; recent logs:" >&2
docker compose -f compose.client.yml logs --tail 30 client >&2 || true
exit 1
