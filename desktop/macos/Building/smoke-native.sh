#!/usr/bin/env bash
# Phase 0 native smoke: probes, state-machine tests and (when a daemon socket exists) the frozen
# Host IPC handshake. Writes every report verbatim under the output directory.
#
# This script is read-only with respect to the machine: it never requests a TCC permission, never
# posts input, never locks/sleeps, never touches the TCC database and never writes inside a
# protected root.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUT_DIR="${1:-${PACKAGE_DIR}/build/smoke}"
CONFIGURATION="${CONFIGURATION:-release}"

mkdir -p "${OUT_DIR}"

echo "== host =="
sw_vers
uname -m
echo "output dir: ${OUT_DIR}"
echo

echo "== build =="
swift build -c "${CONFIGURATION}" --package-path "${PACKAGE_DIR}"
BIN_DIR="$(swift build -c "${CONFIGURATION}" --package-path "${PACKAGE_DIR}" --show-bin-path)"
PROBE="${BIN_DIR}/codebridge-probe"
echo "probe: ${PROBE}"
echo

echo "== unit tests (pure models) =="
swift test --package-path "${PACKAGE_DIR}" 2>&1 | tail -20
echo

echo "== probes (each writes one JSON report) =="
for probe in list permissions signing native-host harness all lock-state input-monitor; do
  echo "-- ${probe}"
  if "${PROBE}" "${probe}" --json --pretty --output "${OUT_DIR}/${probe}.json" >/dev/null; then
    echo "   ok: ${OUT_DIR}/${probe}.json"
  else
    echo "   exit $? (report may still exist): ${OUT_DIR}/${probe}.json"
  fi
done
echo

echo "== handshake smoke (only when the daemon socket exists) =="
SOCKET="${CODEBRIDGE_HOSTIPC_SOCKET:-${HOME}/Library/Application Support/CodeBridge/run/hostipc.sock}"
if [[ -S "${SOCKET}" ]]; then
  echo "socket present: ${SOCKET}"
  set +e
  CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER="${CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER:-1}" \
    "${PROBE}" --handshake-role diagnostics --json --pretty --output "${OUT_DIR}/handshake-diagnostics.json"
  echo "diagnostics handshake exit: $?"
  CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER="${CODEBRIDGE_HOSTIPC_ALLOW_UNVERIFIED_DAEMON_PEER:-1}" \
    "${PROBE}" --handshake-role app --json --pretty --output "${OUT_DIR}/handshake-app.json"
  echo "app handshake exit: $?"
  set -e
else
  echo "no daemon socket at ${SOCKET}; skipping the handshake smoke (start codebridged first)"
fi
echo

echo "== overview =="
for report in "${OUT_DIR}"/*.json; do
  echo "-- ${report}"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$report" <<'PY'
import json, sys
path = sys.argv[1]
with open(path) as handle:
    data = json.load(handle)
if isinstance(data, dict) and "findings" in data:
    for key in ("observed", "untested", "blocked"):
        for entry in data["findings"].get(key, [])[:6]:
            print(f"   {key}: {entry}")
else:
    print("   (handshake or list report; see file)")
PY
  else
    echo "   (python3 unavailable; see file)"
  fi
done

echo
echo "phase0-native smoke complete: ${OUT_DIR}"
