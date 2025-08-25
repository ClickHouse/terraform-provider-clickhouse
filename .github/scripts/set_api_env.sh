#!/usr/bin/env bash

set -euo pipefail

api_env_production=${api_env_production:?"api_env_production not set"}
api_env_staging=${api_env_staging:?"api_env_staging not set"}
api_env_development=${api_env_development:?"api_env_development not set"}

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
ENV=${api_env:-"Production"}

case "${ENV}" in
Production)
  api_url="$(echo "${api_env_production}" | jq -r .api_url)"
  organization_id="$(echo "${api_env_production}" | jq -r .organization_id)"
  api_key_id="$(echo "${api_env_production}" | jq -r .api_key_id)"
  api_key_secret="$(echo "${api_env_production}" | jq -r .api_key_secret)"
  if [[ -n "${cloud:-}" ]]; then
    region="$(echo "${api_env_production}" | jq -rc --arg cloud "${cloud}" '.regions[$cloud]' | jq -c '.[]' | shuf -n 1 | jq -r .)"
    compliance_region="$(echo "${api_env_production}" | jq -rc --arg cloud "${cloud}" '.compliance_regions[$cloud]' | jq -c '.[]' | shuf -n 1 | jq -r .)"
  fi
  ;;

Staging)
  api_url="$(echo "${api_env_staging}" | jq -r .api_url)"
  organization_id="$(echo "${api_env_staging}" | jq -r .organization_id)"
  api_key_id="$(echo "${api_env_staging}" | jq -r .api_key_id)"
  api_key_secret="$(echo "${api_env_staging}" | jq -r .api_key_secret)"
  if [[ -n "${cloud:-}" ]]; then
    region="$(echo "${api_env_staging}" | jq -rc --arg cloud "${cloud}" '.regions[$cloud]' | jq -c '.[]' | shuf -n 1 | jq -r .)"
    compliance_region="$(echo "${api_env_staging}" | jq -rc --arg cloud "${cloud}" '.compliance_regions[$cloud]' | jq -c '.[]' | shuf -n 1 | jq -r .)"
  fi
  ;;

Development)
  api_url="$(echo "${api_env_development}" | jq -r .api_url)"
  organization_id="$(echo "${api_env_development}" | jq -r .organization_id)"
  api_key_id="$(echo "${api_env_development}" | jq -r .api_key_id)"
  api_key_secret="$(echo "${api_env_development}" | jq -r .api_key_secret)"
  if [[ -n "${cloud:-}" ]]; then
    region="$(echo "${api_env_development}" | jq -rc --arg cloud "${cloud}" '.regions[$cloud]' | jq -c '.[]' | shuf -n 1 | jq -r .)"
    compliance_region="$(echo "${api_env_development}" | jq -rc --arg cloud "${cloud}" '.compliance_regions[$cloud]' | jq -c '.[]' | shuf -n 1 | jq -r .)"
  fi
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

  if [[ -z "${region:-}" ]]; then
    echo "Setting default region for ${cloud:-}"
    case "${cloud:-}" in
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
      echo "Got unknown cloud: '${cloud:-}'"
      exit 1
      ;;
    esac

    if [[ -z "${region:-}" ]]; then
      echo "${cloud}_region input must be set when api_env is set to 'Custom'"
      exit 1
    fi

    compliance_region="${region}"

  fi
  ;;
esac

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
if [[ -n "${region:-}" ]]; then
  echo "region=${region}" >>"${GITHUB_OUTPUT}"
  echo "compliance_region=${compliance_region}" >>"${GITHUB_OUTPUT}"
fi
