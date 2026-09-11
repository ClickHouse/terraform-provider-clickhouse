#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf "${test_dir}"' EXIT

mkdir "${test_dir}/bin"
cat > "${test_dir}/bin/shuf" <<'SH'
#!/usr/bin/env bash
if [[ "${TEST_PICK:-first}" == last ]]; then tail -n 1; else head -n 1; fi
SH
chmod +x "${test_dir}/bin/shuf"

credentials='{"api_url":"https://example.invalid/v1","organization_id":"test-org","api_key_id":"test-key","api_key_secret":"secret-sentinel"}'
config='{"regions":{"aws":["us-west-2"],"azure":["westus2"],"gcp":["us-east1","europe-west4"]},"compliance_regions":{"aws":["us-east-1"],"azure":["eastus2"],"gcp":["us-west1"]}}'
legacy="$(jq -c '. + {regions: {aws: ["us-east-2"], gcp: ["us-central1"]}, compliance_regions: {aws: ["us-east-2"], gcp: ["us-central1"]}}' <<< "${credentials}")"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

run_setup() {
  local cloud="$1"
  shift
  : > "${test_dir}/output"
  env -i PATH="${test_dir}/bin:${PATH}" GITHUB_OUTPUT="${test_dir}/output" \
    api_env_production="${legacy}" api_env_staging="${legacy}" api_env_development="${legacy}" \
    "$@" bash "${script_dir}/set_api_env.sh" "${cloud}" \
    > "${test_dir}/stdout" 2> "${test_dir}/stderr"
}

expect_regions() {
  grep -Fqx "region=$1" "${test_dir}/output" || fail "expected region $1"
  grep -Fqx "compliance_region=$2" "${test_dir}/output" || fail "expected compliance region $2"
  grep -Fqx 'api_key_secret=secret-sentinel' "${test_dir}/output" || fail 'credentials changed'
}

for environment in Production Staging Development; do
  suffix="${environment,,}"
  run_setup gcp "api_env=${environment}" "api_env_${suffix}=${credentials}" "example_regions_${suffix}=${config}"
  expect_regions us-east1 us-west1
  if run_setup gcp "api_env=${environment}"; then
    fail 'missing variable used region lists from secrets'
  fi
  [[ ! -s "${test_dir}/output" ]] || fail 'missing variable emitted partial outputs'
  grep -q "EXAMPLE_REGIONS_${environment^^} is required" "${test_dir}/stderr" || fail 'missing variable diagnostic'
  if run_setup gcp "api_env=${environment}" "example_regions_${suffix}="; then
    fail 'empty variable used region lists from secrets'
  fi
done

run_setup gcp "example_regions_production=${config}"
expect_regions us-east1 us-west1
run_setup gcp "example_regions_production=${config}" TEST_PICK=last
expect_regions europe-west4 us-west1
run_setup aws "example_regions_production=${config}"
expect_regions us-west-2 us-east-1
run_setup azure "example_regions_production=${config}"
expect_regions westus2 eastus2

with_extra_fields="$(jq -c '. + {api_key_secret: "must-not-override", api_url: "https://wrong.invalid"}' <<< "${config}")"
run_setup gcp "example_regions_production=${with_extra_fields}"
expect_regions us-east1 us-west1
grep -Fqx 'api_url=https://example.invalid/v1' "${test_dir}/output" || fail 'variable overrode API URL'

with_invalid_secret_regions="$(jq -c '. + {regions: "unused", compliance_regions: null}' <<< "${credentials}")"
run_setup gcp "api_env_production=${with_invalid_secret_regions}" "example_regions_production=${config}"
expect_regions us-east1 us-west1

without_compliance="$(jq -c '.compliance_regions.gcp = []' <<< "${config}")"
run_setup gcp api_env=Development "example_regions_development=${without_compliance}"
expect_regions us-east1 ''

for invalid in 'not-json' ' ' 'null' '[]' '{}' '{"regions":{"gcp":[]}}' \
  '{"regions":{"gcp":["us-east1"]},"compliance_regions":{"gcp":null}}' \
  '{"regions":{"gcp":[42]},"compliance_regions":{"gcp":["us-east1"]}}' \
  '{"regions":{"gcp":["us-east1\ninjected=value"]},"compliance_regions":{"gcp":["us-east1"]}}'; do
  if run_setup gcp "example_regions_production=${invalid}"; then
    fail 'invalid variable used region lists from secrets'
  fi
  [[ ! -s "${test_dir}/output" ]] || fail 'invalid configuration emitted partial outputs'
  grep -q 'Invalid Production region configuration' "${test_dir}/stderr" || fail 'missing validation diagnostic'
done

run_setup '' "api_env_production=${credentials}" example_regions_production=invalid
grep -Fqx 'api_key_secret=secret-sentinel' "${test_dir}/output" || fail 'cleanup lost credentials'
if grep -q '^region=' "${test_dir}/output"; then fail 'cleanup unexpectedly selected a region'; fi

run_setup gcp api_env=Custom api_url=https://example.invalid/v1 organization_id=test-org \
  api_key_id=test-key api_key_secret=secret-sentinel gcp_region=us-central1 \
  api_env_production= api_env_staging= api_env_development= example_regions_production=invalid
expect_regions us-central1 us-central1

if run_setup gcp api_env=Unknown; then fail 'unknown environment accepted'; fi

echo 'Example region configuration checks passed.'
