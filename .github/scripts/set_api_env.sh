#!/usr/bin/env bash

set -euo pipefail

api_url=${api_url:-""}
organization_id=${organization_id:-""}
api_key_id=${api_key_id:-""}
api_key_secret=${api_key_secret:-""}
aws_region=${aws_region:-""}
azure_region=${azure_region:-""}
gcp_region=${gcp_region:-""}

cloud="${1:-""}"
region=""
compliance_region=""

# When this script is called by the cron schedule inputs are empty so we default to Production.
api_environment=${api_env:-"Production"}

select_region() {
  local candidates
  if ! candidates="$(jq -ce --arg kind "$1" --arg cloud "${cloud}" '
    .[$kind][$cloud]
    | if type == "array" and ($kind == "compliance_regions" or length > 0)
        and all(.[]; type == "string" and test("^[a-z0-9-]+$"))
      then .
      else error("Invalid region list")
      end
  ' <<< "${region_config}" 2>/dev/null)"; then
    echo "Invalid ${api_environment} region configuration: $1.${cloud} must be an array of region names (regions cannot be empty)" >&2
    return 1
  fi
  jq -r '.[]' <<< "${candidates}" | shuf -n 1
}

case "${api_environment}" in
Production)
  api_config=${api_env_production:?"api_env_production not set"}
  region_config=${example_regions_production:-""}
  ;;

Staging)
  api_config=${api_env_staging:?"api_env_staging not set"}
  region_config=${example_regions_staging:-""}
  ;;

Development)
  api_config=${api_env_development:?"api_env_development not set"}
  region_config=${example_regions_development:-""}
  ;;

Custom)
  if [[ -z "${api_url:-}" ]]; then
    echo "api_url input must be set when api_env is set to 'Custom'"
    exit 1
  fi

  if [[ -z "${organization_id:-}" ]]; then
    echo "organization_id input must be set when api_env is set to 'Custom'"
    exit 1
  fi

  if [[ -z "${api_key_id:-}" ]]; then
    echo "api_key_id input must be set when api_env is set to 'Custom'"
    exit 1
  fi

  if [[ -z "${api_key_secret:-}" ]]; then
    echo "api_key_secret input must be set when api_env is set to 'Custom'"
    exit 1
  fi

  if [[ -z "${region}" ]]; then
    echo "Setting default region for ${cloud}"
    case "${cloud}" in
    aws)
      region="${aws_region}"
      ;;
    azure)
      region="${azure_region}"
      ;;
    gcp)
      region="${gcp_region}"
      ;;
    *)
      echo "Got unknown cloud: '${cloud}'"
      exit 1
      ;;
    esac

    if [[ -z "${region}" ]]; then
      echo "${cloud}_region input must be set when api_env is set to 'Custom'"
      exit 1
    fi

    compliance_region="${region}"

  fi
  ;;
*)
  echo "Unknown API environment: ${api_environment}" >&2
  exit 1
  ;;
esac

if [[ "${api_environment}" != "Custom" ]]; then
  api_url="$(jq -r .api_url <<< "${api_config}")"
  organization_id="$(jq -r .organization_id <<< "${api_config}")"
  api_key_id="$(jq -r .api_key_id <<< "${api_config}")"
  api_key_secret="$(jq -r .api_key_secret <<< "${api_config}")"
  if [[ -n "${cloud}" ]]; then
    if [[ -z "${region_config}" ]]; then
      echo "EXAMPLE_REGIONS_${api_environment^^} is required when selecting example regions" >&2
      exit 1
    fi
    region="$(select_region regions)"
    compliance_region="$(select_region compliance_regions)"
  fi
fi

# shellcheck disable=SC2129
echo "api_url=${api_url}" >>"${GITHUB_OUTPUT}"

echo "organization_id=${organization_id}" >>"${GITHUB_OUTPUT}"
echo "::add-mask::${organization_id}"

echo "api_key_id=${api_key_id}" >>"${GITHUB_OUTPUT}"
echo "::add-mask::${api_key_id}"

echo "api_key_secret=${api_key_secret}" >>"${GITHUB_OUTPUT}"
echo "::add-mask::${api_key_secret}"

echo "region='${region}'"
echo "compliance_region='${compliance_region}'"
if [[ -n "${region}" ]]; then
  echo "region=${region}" >>"${GITHUB_OUTPUT}"
  echo "compliance_region=${compliance_region}" >>"${GITHUB_OUTPUT}"
fi
