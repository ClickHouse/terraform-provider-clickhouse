#!/usr/bin/env bash

set -euo pipefail

# Emit only region maps, never the credentials from API_ENV_*.
if ! config="$(jq -e '
  {regions, compliance_regions}
  | if all(.[]; type == "object"
      and all(.[]; type == "array"
        and all(.[]; type == "string" and test("^[a-z0-9-]+$"))))
    then .
    else error("Invalid region configuration")
    end
' 2>/dev/null)"; then
  echo "Cannot export region configuration: expected maps of region-name arrays" >&2
  exit 1
fi
printf '%s\n' "${config}"
