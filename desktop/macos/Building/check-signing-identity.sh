#!/usr/bin/env bash
# Read-only report of the code-signing identities and expected signing configuration on this host.
#
# Writes nothing, requests no permission, touches no TCC database.
set -euo pipefail

echo "== host =="
sw_vers
uname -m
echo

echo "== code-signing identities (security find-identity -v -p codesigning) =="
identities="$(security find-identity -v -p codesigning 2>&1)"
echo "${identities}"
if [[ "${identities}" == *"0 valid identities found"* ]]; then
  echo "SIGNED_TCC_BLOCKED_EXTERNAL: no genuine signing identity with private key"
fi
echo

echo "== environment =="
for name in CODEBRIDGE_TEAM_ID CODEBRIDGE_SIGNING_IDENTITY CODEBRIDGE_EXPECT_TEAM_ID \
            CODEBRIDGE_EXPECT_APP_SIGNING_ID CODEBRIDGE_EXPECT_DAEMON_SIGNING_ID \
            CODEBRIDGE_PHASE0_DEBUG CODEBRIDGE_PHASE0_PROBE CODEBRIDGE_HOSTIPC_SOCKET; do
  value="${!name:-}"
  if [[ -n "${value}" ]]; then
    echo "${name}=${value}"
  else
    echo "${name}=<unset>"
  fi
done
echo

echo "== launchd gui domain =="
uid="$(id -u)"
echo "gui/${uid} exists: $(launchctl print "gui/${uid}" >/dev/null 2>&1 && echo yes || echo no)"
echo

if [[ $# -gt 0 ]]; then
  for binary in "$@"; do
    echo "== ${binary} =="
    if [[ -e "${binary}" ]]; then
      codesign -dv --verbose=4 "${binary}" 2>&1 || true
      codesign -d -r- "${binary}" 2>&1 || true
      codesign --verify --deep --strict --verbose=4 "${binary}" 2>&1 || true
      [[ -d "${binary}" ]] || shasum -a 256 "${binary}"
    else
      echo "missing"
    fi
    echo
  done
fi

cat <<'EOF'
Reminder: with zero valid code-signing identities, every TCC / SMAppService / LaunchAgent /
protected-root outcome stays BLOCKED (missing prerequisite). That is not architecture evidence
that protected roots are unsupported.
EOF
