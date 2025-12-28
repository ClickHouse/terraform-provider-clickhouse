#!/usr/bin/env bash

set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:?"SERVICE_NAME cannot be empty"}"

### Resource Group
echo "::group::Deleting Resource Group..."

if az group show --name "${SERVICE_NAME}" >/dev/null 2>&1; then
  az group delete --name "${SERVICE_NAME}" --yes
else
  echo "Resource group '${SERVICE_NAME}' does not exist, skipping deletion."
fi

echo "::endgroup::"
