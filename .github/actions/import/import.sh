#!/usr/bin/env bash

set -euo pipefail

RESOURCE_TYPE="${RESOURCE_TYPE:?"RESOURCE_TYPE cannot be empty"}"
API_URL="${API_URL:?"API_URL cannot be empty"}"

echo "::group::Run terraform destroy"

cd "tests/import/${RESOURCE_TYPE}"

if [ "${API_URL}" != "" ]; then
  export CLICKHOUSE_API_URL="${API_URL}"
  echo "Using '$CLICKHOUSE_API_URL' as API URL"
fi

if [[ "${ACTIONS_RUNNER_DEBUG:-}" == "true" ]] || [[ "${ACTIONS_STEP_DEBUG:-}" == "true" ]]; then
  export TF_LOG="debug"
fi

terraform destroy -no-color -auto-approve -var-file=variables.tfvars

echo "::endgroup::"
