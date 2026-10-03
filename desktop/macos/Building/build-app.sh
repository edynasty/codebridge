#!/usr/bin/env bash
# Builds and assembles CodeBridge.app (Phase 0 host adapter).
#
# Signing policy: a genuine signing identity and Team ID are REQUIRED for any bundle that is used
# for TCC, SMAppService or LaunchAgent evidence. `--unsigned-dev` produces a bundle that is
# explicitly marked as not valid for that evidence; it never signs with an ad-hoc identity to
# pretend a TCC-valid artifact exists.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${PACKAGE_DIR}/../.." && pwd)"

BUNDLE_ID="com.codebridge.app"
VERSION="0.1.0"
BUILD_NUMBER="phase0"
CONFIGURATION="release"
SIGN_IDENTITY="${CODEBRIDGE_SIGNING_IDENTITY:-}"
TEAM_ID="${CODEBRIDGE_TEAM_ID:-}"
UNSIGNED_DEV=0
DAEMON_BINARY=""
EMBED_PHASE0_ENV=0
OUTPUT_DIR="${PACKAGE_DIR}/build"
APP_NAME="CodeBridge"
DISPLAY_NAME=""

usage() {
  cat <<'EOF'
usage: build-app.sh [options]

  --sign-identity <id>     Apple Development (preferred) or Developer ID Application identity
                           (or CODEBRIDGE_SIGNING_IDENTITY); defaults to an available identity
  --team-id <TEAMID>       optional expected Team ID; actual Team is read from a signed executable
                           (or CODEBRIDGE_TEAM_ID); a mismatch is refused before assembly
  --daemon <path>          codebridged binary to embed at Contents/MacOS/codebridged
  --embed-phase0-env       bake CODEBRIDGE_PHASE0_DEBUG/CODEBRIDGE_PHASE0_PROBE/HOSTIPC socket into
                           the LaunchAgent plist (development/attribution builds only)
  --display-name <name>    override App UI name only for signed --embed-phase0-env acceptance;
                           product default, bundle path, executable and identities stay unchanged
  --unsigned-dev           assemble without a signing identity; the bundle is marked and is NOT
                           valid for TCC / SMAppService / LaunchAgent evidence
  --bundle-id <id>         default com.codebridge.app
  --version <v>            default 0.1.0
  --build-number <n>       default phase0
  --configuration <c>      release (default) or debug
  --output-dir <dir>       default desktop/macos/build
  -h, --help               this text

Observed identities on this host: run Building/check-signing-identity.sh
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sign-identity) SIGN_IDENTITY="${2:-}"; shift 2 ;;
    --team-id) TEAM_ID="${2:-}"; shift 2 ;;
    --daemon) DAEMON_BINARY="${2:-}"; shift 2 ;;
    --embed-phase0-env) EMBED_PHASE0_ENV=1; shift ;;
    --display-name) DISPLAY_NAME="${2:?--display-name requires a name}"; shift 2 ;;
    --unsigned-dev) UNSIGNED_DEV=1; shift ;;
    --bundle-id) BUNDLE_ID="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --build-number) BUILD_NUMBER="${2:-}"; shift 2 ;;
    --configuration) CONFIGURATION="${2:-}"; shift 2 ;;
    --output-dir) OUTPUT_DIR="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -n "${DISPLAY_NAME}" && ( "${EMBED_PHASE0_ENV}" -ne 1 || "${UNSIGNED_DEV}" -ne 0 ) ]]; then
  echo "error: --display-name requires a signed --embed-phase0-env acceptance build" >&2
  exit 2
fi

if [[ -n "${DAEMON_BINARY}" && ! -x "${DAEMON_BINARY}" ]]; then
  echo "error: --daemon ${DAEMON_BINARY} is not an executable" >&2
  exit 2
fi

IDENTITY_WORK=""
ENV_FILE=""
trap '[[ -z "${IDENTITY_WORK}" ]] || rm -rf "${IDENTITY_WORK}"; [[ -z "${ENV_FILE}" ]] || rm -f "${ENV_FILE}"' EXIT
echo "== observed code-signing identities =="
IDENTITIES="$(security find-identity -v -p codesigning 2>&1)" || {
  echo "${IDENTITIES}" >&2
  echo "SIGNED_TCC_BLOCKED_EXTERNAL: cannot enumerate signing identities" >&2
  exit 2
}
echo "${IDENTITIES}"
if [[ "${UNSIGNED_DEV}" -eq 0 ]]; then
  if [[ "${IDENTITIES}" == *"0 valid identities found"* ]]; then
    echo "SIGNED_TCC_BLOCKED_EXTERNAL: install an Apple Development or Developer ID Application certificate with its private key" >&2
    exit 2
  fi
  if [[ -z "${SIGN_IDENTITY}" ]]; then
    for kind in "Apple Development:" "Developer ID Application:"; do
      while IFS= read -r line; do
        if [[ "${line}" == *"${kind}"* && "${line}" =~ ([A-Fa-f0-9]{40}) ]]; then
          SIGN_IDENTITY="${BASH_REMATCH[1]}"
          break
        fi
      done <<< "${IDENTITIES}"
      [[ -z "${SIGN_IDENTITY}" ]] || break
    done
  fi
  if [[ -z "${SIGN_IDENTITY}" || "${SIGN_IDENTITY}" == "-" || "${BUNDLE_ID}" != "com.codebridge.app" ]]; then
    echo "error: genuine Apple identity and stable com.codebridge.app identifier required" >&2
    exit 2
  fi
  # Validate the selected private key, Apple trust and actual Team before touching build output.
  IDENTITY_WORK="$(mktemp -d "${TMPDIR:-/tmp}/codebridge-signing.XXXXXX")"
  cp /usr/bin/true "${IDENTITY_WORK}/identity-check"
  codesign --force --sign "${SIGN_IDENTITY}" --identifier com.codebridge.identity-check \
    "${IDENTITY_WORK}/identity-check"
  codesign --verify --strict --test-requirement '=anchor apple generic' "${IDENTITY_WORK}/identity-check"
  IDENTITY_METADATA="$(codesign -dv --verbose=4 "${IDENTITY_WORK}/identity-check" 2>&1)"
  if [[ "${IDENTITY_METADATA}" != *"Authority=Apple Development:"* && \
        "${IDENTITY_METADATA}" != *"Authority=Developer ID Application:"* ]]; then
    echo "error: only Apple Development or Developer ID Application identities are accepted" >&2
    exit 2
  fi
  ACTUAL_TEAM="$(printf '%s\n' "${IDENTITY_METADATA}" | sed -n 's/^TeamIdentifier=//p')"
  if [[ -z "${ACTUAL_TEAM}" || "${ACTUAL_TEAM}" == "not set" || \
        ( -n "${TEAM_ID}" && "${TEAM_ID}" != "${ACTUAL_TEAM}" ) ]]; then
    echo "error: missing actual Team ID or configured Team ID differs from signed metadata" >&2
    exit 2
  fi
  TEAM_ID="${ACTUAL_TEAM}"
else
  # A development assembly has no trusted Team, irrespective of environment configuration.
  TEAM_ID=""
fi
echo
echo "== build =="
swift build -c "${CONFIGURATION}" --package-path "${PACKAGE_DIR}"
BIN_DIR="$(swift build -c "${CONFIGURATION}" --package-path "${PACKAGE_DIR}" --show-bin-path)"

APP_BUNDLE="${OUTPUT_DIR}/${APP_NAME}.app"
rm -rf "${APP_BUNDLE}"
mkdir -p "${APP_BUNDLE}/Contents/MacOS"
mkdir -p "${APP_BUNDLE}/Contents/Resources"
mkdir -p "${APP_BUNDLE}/Contents/Library/LaunchAgents"

cp "${BIN_DIR}/CodeBridge" "${APP_BUNDLE}/Contents/MacOS/CodeBridge"
cp "${BIN_DIR}/codebridge-probe" "${APP_BUNDLE}/Contents/MacOS/codebridge-probe"
chmod +x "${APP_BUNDLE}/Contents/MacOS/CodeBridge" "${APP_BUNDLE}/Contents/MacOS/codebridge-probe"

DAEMON_EMBEDDED="false"
if [[ -n "${DAEMON_BINARY}" ]]; then
  # Executability was checked before assembly.
  cp "${DAEMON_BINARY}" "${APP_BUNDLE}/Contents/MacOS/codebridged"
  chmod +x "${APP_BUNDLE}/Contents/MacOS/codebridged"
  DAEMON_EMBEDDED="true"
fi

UNSIGNED_MARKER="false"
if [[ "${UNSIGNED_DEV}" -eq 1 ]]; then
  UNSIGNED_MARKER="true"
  cat > "${APP_BUNDLE}/Contents/Resources/UNSIGNED-DEV-BUILD.txt" <<'EOF'
UNSIGNED DEV BUILD
This bundle was assembled without a Developer ID / Apple Development identity. No trusted Team ID
is configured. Binaries may carry the toolchain's automatic ad-hoc signature, which is NOT a
genuine identity and is NOT valid for:
  - SMAppService / LaunchAgent registration evidence
  - Screen Recording / Accessibility / Input Monitoring attribution evidence
  - Files and Folders (protected root) authorization evidence
Do not cite this build as TCC, LaunchAgent or attribution evidence.
EOF
fi
BUILD_KIND="signed"
[[ "${UNSIGNED_DEV}" -eq 0 ]] || BUILD_KIND="unsigned-dev"

sed \
  -e "s|__BUNDLE_ID__|${BUNDLE_ID}|g" \
  -e "s|__VERSION__|${VERSION}|g" \
  -e "s|__BUILD__|${BUILD_NUMBER}|g" \
  -e "s|__BUILD_KIND__|${BUILD_KIND}|g" \
  -e "s|__TEAM_ID__|${TEAM_ID}|g" \
  -e "s|__UNSIGNED_MARKER_VALUE__|${UNSIGNED_MARKER}|g" \
  "${SCRIPT_DIR}/Info.plist.in" > "${APP_BUNDLE}/Contents/Info.plist"

if [[ -n "${DISPLAY_NAME}" ]]; then
  plutil -replace CFBundleName -string "${DISPLAY_NAME}" "${APP_BUNDLE}/Contents/Info.plist"
  plutil -insert CFBundleDisplayName -string "${DISPLAY_NAME}" "${APP_BUNDLE}/Contents/Info.plist"
fi

ENV_FILE="$(mktemp)"
cat > "${ENV_FILE}" <<EOF
	<key>EnvironmentVariables</key>
	<dict>
		<key>CODEBRIDGE_APP_TEAM_ID</key>
		<string>${TEAM_ID}</string>
		<key>CODEBRIDGE_APP_BUNDLE_ID</key>
		<string>${BUNDLE_ID}</string>
EOF
if [[ "${EMBED_PHASE0_ENV}" -eq 1 ]]; then
  cat >> "${ENV_FILE}" <<EOF
		<key>CODEBRIDGE_DAEMON_VERSION</key>
		<string>phase0b</string>
		<key>CODEBRIDGE_PHASE0_DEBUG</key>
		<string>1</string>
		<key>CODEBRIDGE_PHASE0_PROBE</key>
		<string>${APP_BUNDLE}/Contents/MacOS/codebridge-probe</string>
		<key>CODEBRIDGE_HOSTIPC_SOCKET</key>
		<string>${HOME}/Library/Application Support/CodeBridge/run/hostipc.sock</string>
EOF
fi
printf '\t</dict>\n' >> "${ENV_FILE}"

awk -v envfile="${ENV_FILE}" '{
  if ($0 == "__ENVIRONMENT_SECTION__") {
    while ((getline line < envfile) > 0) { print line }
  } else { print }
}' "${SCRIPT_DIR}/com.codebridge.daemon.plist.in" \
  | sed -e "s|__LABEL__|com.codebridge.daemon|g" \
  > "${APP_BUNDLE}/Contents/Library/LaunchAgents/com.codebridge.daemon.plist"
rm -f "${ENV_FILE}"

plutil -lint "${APP_BUNDLE}/Contents/Info.plist"
plutil -lint "${APP_BUNDLE}/Contents/Library/LaunchAgents/com.codebridge.daemon.plist"

SIGNING_MODE="unsigned-dev"
if [[ "${UNSIGNED_DEV}" -eq 0 ]]; then
  SIGNING_MODE="signed"
  echo
  echo "== sign (nested first) =="
  codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" \
    --identifier "com.codebridge.probe" "${APP_BUNDLE}/Contents/MacOS/codebridge-probe"
  if [[ "${DAEMON_EMBEDDED}" == "true" ]]; then
    codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" \
      --identifier "com.codebridge.daemon" "${APP_BUNDLE}/Contents/MacOS/codebridged"
  fi
  codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" \
    --identifier "${BUNDLE_ID}" "${APP_BUNDLE}"
fi

echo
echo "== signing tree (observed) =="
for binary in "${APP_BUNDLE}/Contents/MacOS/codebridge-probe" \
              "${APP_BUNDLE}/Contents/MacOS/codebridged" \
              "${APP_BUNDLE}"; do
  if [[ -e "${binary}" ]]; then
    echo "-- ${binary}"
    codesign -dv --verbose=4 "${binary}" 2>&1
    codesign -d -r- "${binary}" 2>&1
    if [[ "${UNSIGNED_DEV}" -eq 0 ]]; then
      case "${binary}" in
        */codebridge-probe) expected_id="com.codebridge.probe" ;;
        */codebridged) expected_id="com.codebridge.daemon" ;;
        *) expected_id="com.codebridge.app" ;;
      esac
      codesign --verify --deep --strict --verbose=4 --test-requirement \
        "=anchor apple generic and identifier \"${expected_id}\" and certificate leaf[subject.OU] = \"${TEAM_ID}\"" "${binary}"
    fi
    [[ -d "${binary}" ]] || shasum -a 256 "${binary}"
  fi
done

echo
echo "== result =="
echo "app bundle: ${APP_BUNDLE}"
echo "signing mode: ${SIGNING_MODE}"
echo "team id: ${TEAM_ID:-<none>}"
echo "daemon embedded: ${DAEMON_EMBEDDED}"
if [[ "${UNSIGNED_DEV}" -eq 1 ]]; then
  cat <<'EOF'

WARNING: unsigned development build.
  It is NOT valid for TCC, SMAppService or LaunchAgent evidence.
  Rebuild with --sign-identity "<genuine identity>" --team-id <TEAMID> for those spikes.
EOF
fi
