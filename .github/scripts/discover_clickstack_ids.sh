#!/usr/bin/env bash
# Finds the e2e ClickStack service and its platform-created connection id, and
# appends both to the example's variables.tfvars.
#
# Neither is a secret. The service is looked up by name, so rebuilding it needs
# nothing updated here; the connection is minted per service by the platform, so
# its id has to be read live.
#
# Reads ORGANIZATION_ID, TOKEN_KEY, TOKEN_SECRET and optionally CLICKHOUSE_API_URL
# from the environment. Takes the target variables.tfvars as $1.
set -euo pipefail

VAR_FILE="${1:?usage: discover_clickstack_ids.sh <path-to-variables.tfvars>}"
API_URL="${CLICKHOUSE_API_URL:-https://api.clickhouse.cloud/v1}"
ORG_URL="${API_URL}/organizations/${ORGANIZATION_ID}"

# Created once per org by hand, with managed ClickStack onboarded — see
# examples/full/clickstack/aws/README.md. The name carries no per-run token, so
# cleanup.sh does not match it.
SERVICE_NAME="[e2e]-clickstack-nightly"

# Bounded and retried: a flaky call would otherwise abort the whole e2e run, and
# this step is not inside the retry wrapper the terraform steps use. --fail
# because a retry that appended a second response body to the first would leave
# jq reading whichever document came last.
CURL_OPTS=(-sS --fail --connect-timeout 10 --max-time 60 --retry 3 --retry-delay 2)

api() { curl "${CURL_OPTS[@]}" --user "${TOKEN_KEY}:${TOKEN_SECRET}" "$@"; }

services=$(api "${ORG_URL}/services") || {
  echo "::error::Could not list services in organization ${ORGANIZATION_ID}." >&2
  exit 1
}

service_id=$(jq -r --arg name "$SERVICE_NAME" \
  '[(.result // [])[] | select(.name == $name) | .id] | first // empty' <<<"$services")

if [ -z "$service_id" ]; then
  echo "::error::No service named ${SERVICE_NAME} in organization ${ORGANIZATION_ID}. It has to exist with managed ClickStack onboarded; see examples/full/clickstack/aws/README.md." >&2
  exit 1
fi

# Connections are not exposed on Cloud. The platform creates exactly one per
# service pointing at itself, so read its id off the sources onboarding created.
# `// empty` matters: jq -r prints a JSON null as the string "null", which would
# sail past the emptiness check below and land "null" in variables.tfvars.
sources=$(api "${ORG_URL}/services/${service_id}/clickstack/sources") || {
  echo "::error::Could not read ${SERVICE_NAME}'s ClickStack sources." >&2
  exit 1
}
connection_id=$(jq -r '[.result[].connection // empty] | unique | if length == 1 then .[0] else empty end' <<<"$sources")

if [ -z "$connection_id" ]; then
  echo "::error::Could not read a single connection id off ${SERVICE_NAME}'s sources. ClickStack creates its default sources once telemetry arrives, so this usually means the service has never ingested anything." >&2
  exit 1
fi

echo "Using ClickStack service ${SERVICE_NAME} (${service_id}), connection ${connection_id}"

cat >>"$VAR_FILE" <<EOF

clickstack_service_id = "${service_id}"
connection_id         = "${connection_id}"
EOF
