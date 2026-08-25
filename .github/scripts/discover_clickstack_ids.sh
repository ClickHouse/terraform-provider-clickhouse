#!/usr/bin/env bash
# Discovers the Cloud service running managed ClickStack and its platform-created
# connection id, and appends both to the example's variables.tfvars.
#
# Neither is held as a secret. The connection is minted per service by the
# platform, so a stored id goes stale the moment the service is recreated; the
# service is identified by which one answers on the ClickStack route rather than
# by name or id, so nothing has to be updated when it is rebuilt.
#
# Reads ORGANIZATION_ID, TOKEN_KEY, TOKEN_SECRET and optionally CLICKHOUSE_API_URL
# from the environment. Takes the target variables.tfvars as $1.
set -euo pipefail

VAR_FILE="${1:?usage: discover_clickstack_ids.sh <path-to-variables.tfvars>}"
API_URL="${CLICKHOUSE_API_URL:-https://api.clickhouse.cloud/v1}"
ORG_URL="${API_URL}/organizations/${ORGANIZATION_ID}"

# Bounded and retried: a flaky probe would otherwise abort the whole e2e run,
# and this step is not inside the retry wrapper the terraform steps use.
CURL_OPTS=(-sS --connect-timeout 10 --max-time 60 --retry 3 --retry-delay 2 --retry-all-errors)

api() { curl "${CURL_OPTS[@]}" --user "${TOKEN_KEY}:${TOKEN_SECRET}" "$@"; }

services=$(api "${ORG_URL}/services") || {
  echo "::error::Could not list services in organization ${ORGANIZATION_ID}." >&2
  exit 1
}
# Distinguishes an auth/API failure from a genuinely empty org — without this the
# loop below just sees no candidates and reports "nothing onboarded".
jq -e 'has("result")' <<<"$services" >/dev/null || {
  echo "::error::Unexpected response listing services: ${services}" >&2
  exit 1
}

# A service without ClickStack answers 403 ("ClickStack has not been setup for
# this service"), so the status code is what identifies the right one. State is
# not a useful filter: idle services serve ClickStack fine, and stopped ones 403
# like any other.
service_id=""
while IFS=$'\t' read -r id name; do
  [ -n "$id" ] || continue
  code=$(api -o /dev/null -w '%{http_code}' "${ORG_URL}/services/${id}/clickstack/sources")
  case "$code" in
    200) ;;
    403) continue ;;
    *)
      echo "::error::Probing service ${name} (${id}) returned ${code}; expected 200 (onboarded) or 403 (not onboarded)." >&2
      exit 1
      ;;
  esac
  if [ -n "$service_id" ]; then
    echo "::error::More than one service has ClickStack onboarded (${name} and ${service_name}). The e2e run cannot pick between them." >&2
    exit 1
  fi
  service_id="$id"
  service_name="$name"
done < <(jq -r '.result[] | "\(.id)\t\(.name)"' <<<"$services")

if [ -z "$service_id" ]; then
  echo "::error::No service in this organization has ClickStack onboarded. Onboarding is a console step the provider cannot do." >&2
  exit 1
fi

# Connections are not exposed on Cloud. The platform creates exactly one per
# service pointing at itself, so read its id off the sources onboarding created.
# `// empty` matters: jq -r prints a JSON null as the string "null", which would
# sail past the emptiness check below and land "null" in variables.tfvars.
connection_id=$(api "${ORG_URL}/services/${service_id}/clickstack/sources" \
  | jq -r '[.result[].connection // empty] | unique | if length == 1 then .[0] else empty end')

if [ -z "$connection_id" ]; then
  echo "::error::Could not read a single connection id off ${service_name}'s sources. ClickStack creates its default sources once telemetry arrives, so this usually means the service has never ingested anything." >&2
  exit 1
fi

echo "Using ClickStack service ${service_name} (${service_id}), connection ${connection_id}"

cat >>"$VAR_FILE" <<EOF

clickstack_service_id = "${service_id}"
connection_id         = "${connection_id}"
EOF
