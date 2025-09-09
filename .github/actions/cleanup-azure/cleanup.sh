#!/usr/bin/env bash

set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:?"SERVICE_NAME cannot be empty"}"

### Resource Group
echo "::group::Deleting Resource Group..."
az group delete --name "${SERVICE_NAME}" --yes
echo "::endgroup::"
