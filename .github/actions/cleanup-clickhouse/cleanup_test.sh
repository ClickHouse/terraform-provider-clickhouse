#!/usr/bin/env bash
# Runs cleanup.sh against a stubbed curl (no network, no credentials) and
# checks how the service sweep handles deletion protection.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

mkdir "${WORK}/bin"
cat >"${WORK}/bin/curl" <<'STUB'
#!/usr/bin/env bash
# One stopped service, svc1, that refuses DELETE with 409 while protected.
method=GET url="" data="" write_code=false fail=false
while [[ $# -gt 0 ]]; do
  case "$1" in
  -X) method="$2"; shift 2 ;;
  --data | -d) data="$2"; shift 2 ;;
  -w) write_code=true; shift 2 ;;
  --fail) fail=true; shift ;;
  -o | -H | --user | --connect-timeout | --max-time) shift 2 ;;
  http*) url="$1"; shift ;;
  *) shift ;;
  esac
done
path="${url#"${API_URL}"}"
echo "${method} ${path} ${data}" >>"${STUB_DIR}/calls"

code=200
body='{"result":[]}'
case "${method} ${path}" in
"GET /organizations/org/services")
  if [[ ! -e "${STUB_DIR}/deleted" ]]; then
    protection=',"deletionProtection":true'
    if [[ -e "${STUB_DIR}/unprotected" ]]; then protection=',"deletionProtection":false'; fi
    if [[ -n "${STUB_NO_PROTECTION_FIELD:-}" ]]; then protection=""; fi
    body="{\"result\":[{\"id\":\"svc1\",\"name\":\"svc-${SUFFIX}\",\"state\":\"stopped\",\"isPrimary\":true${protection}}]}"
  fi
  ;;
"GET /organizations/org")
  body='{"result":{"privateEndpoints":[]}}'
  ;;
"PATCH /organizations/org/services/svc1")
  if [[ -n "${STUB_UNPROTECT_FAILS:-}" ]]; then code=500; else touch "${STUB_DIR}/unprotected"; fi
  ;;
"DELETE /organizations/org/services/svc1")
  if [[ -e "${STUB_DIR}/unprotected" || -n "${STUB_NO_PROTECTION_FIELD:-}" ]]; then touch "${STUB_DIR}/deleted"; else code=409; fi
  ;;
esac

if [[ "${write_code}" == true ]]; then
  printf '%s' "${code}"
  exit 0
fi
if [[ "${fail}" == true && "${code}" -ge 400 ]]; then
  exit 22
fi
printf '%s' "${body}"
STUB
chmod +x "${WORK}/bin/curl"

export PATH="${WORK}/bin:${PATH}"
export API_URL="http://stub/v1" ORGANIZATION_ID=org TOKEN_KEY=key TOKEN_SECRET=secret SUFFIX=run1
export HTTP_ATTEMPTS=1 RETRY_DELAY_SECONDS=0

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# run_sweep <case-dir> [VAR=value ...]: runs the sweep with its calls and output kept in <case-dir>.
run_sweep() {
  local dir="$1"
  shift
  mkdir -p "${dir}"
  touch "${dir}/calls"
  env STUB_DIR="${dir}" "$@" "${BASH}" "${HERE}/cleanup.sh" >"${dir}/out" 2>&1
}

echo "--- a protected service is unprotected, then deleted"
dir="${WORK}/unprotect"
run_sweep "${dir}" SWEEP_TIMEOUT_SECONDS=5 POLL_INTERVAL_SECONDS=0 || { cat "${dir}/out" >&2; fail "sweep failed"; }
patch_at=$(grep -n '^PATCH /organizations/org/services/svc1 {"deletionProtection": false}' "${dir}/calls" | head -1 | cut -d: -f1 || true)
delete_at=$(grep -n '^DELETE /organizations/org/services/svc1 ' "${dir}/calls" | head -1 | cut -d: -f1 || true)
[[ -n "${patch_at}" ]] || { cat "${dir}/out" >&2; fail "no unprotect PATCH was sent"; }
[[ -n "${delete_at}" ]] || fail "no DELETE was sent"
[[ "${patch_at}" -lt "${delete_at}" ]] || fail "DELETE was sent before the unprotect PATCH"
[[ -e "${dir}/deleted" ]] || fail "service was not deleted"
if grep -q '::warning::' "${dir}/out"; then fail "unexpected warning: $(grep '::warning::' "${dir}/out")"; fi

echo "--- a service that does not report protection is deleted without an unprotect PATCH"
dir="${WORK}/no-field"
run_sweep "${dir}" STUB_NO_PROTECTION_FIELD=1 SWEEP_TIMEOUT_SECONDS=5 POLL_INTERVAL_SECONDS=0 || { cat "${dir}/out" >&2; fail "sweep failed"; }
if grep -q '^PATCH ' "${dir}/calls"; then fail "unexpected PATCH: $(grep '^PATCH ' "${dir}/calls")"; fi
[[ -e "${dir}/deleted" ]] || fail "service was not deleted"

echo "--- a failing unprotect warns once and is retried every pass"
dir="${WORK}/unprotect-fails"
# No poll delay, so many passes fit before the deadline even on a slow runner.
if run_sweep "${dir}" STUB_UNPROTECT_FAILS=1 SWEEP_TIMEOUT_SECONDS=3 POLL_INTERVAL_SECONDS=0; then
  fail "sweep succeeded although the service could not be deleted"
fi
warnings=$(grep -c '::warning::could not turn off deletion protection on service svc1' "${dir}/out" || true)
patches=$(grep -c '^PATCH /organizations/org/services/svc1 ' "${dir}/calls" || true)
[[ "${warnings}" -eq 1 ]] || fail "want 1 warning, got ${warnings}"
[[ "${patches}" -ge 2 ]] || fail "want the unprotect PATCH on every pass, got ${patches}"

echo "cleanup sweep tests passed"
