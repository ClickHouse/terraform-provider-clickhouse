#!/usr/bin/env bash

set -euo pipefail

API_URL="${API_URL:-"https://api.clickhouse.cloud/v1"}"
ORGANIZATION_ID="${ORGANIZATION_ID:?"ORGANIZATION_ID cannot be empty"}"
TOKEN_KEY="${TOKEN_KEY:?"TOKEN_KEY cannot be empty"}"
TOKEN_SECRET="${TOKEN_SECRET:?"TOKEN_SECRET cannot be empty"}"
SUFFIX="${SUFFIX:?"SUFFIX cannot be empty"}"

ORG_URL="${API_URL}/organizations/${ORGANIZATION_ID}"

# How long a sweep may poll before giving up and naming what leaked. The
# provider itself waits at most 10 minutes for a service to reach `stopped`
# (see WaitForServiceState in internal/api/service.go), so 20 leaves room for
# the stop-then-delete round trip. Anything still standing after that is stuck,
# and waiting longer only burns runner time -- an earlier version of this script
# had no bound at all and spun for three and a half hours.
SWEEP_TIMEOUT_SECONDS="${SWEEP_TIMEOUT_SECONDS:-1200}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-5}"

# Report what the sweep would touch without touching it. Handy for checking a
# suffix against a real organization before letting the deletes run.
DRY_RUN="${DRY_RUN:-false}"

HTTP_ATTEMPTS="${HTTP_ATTEMPTS:-5}"
RETRY_DELAY_SECONDS="${RETRY_DELAY_SECONDS:-5}"

# Deliberately no --retry: curl appends a retried body to the partial one it has
# already written, so a truncated response reaches jq as two concatenated
# documents. Retrying in bash keeps exactly one whole body per attempt.
CURL_OPTS=(-sS --connect-timeout 10 --max-time 60)

api() { curl "${CURL_OPTS[@]}" --user "${TOKEN_KEY}:${TOKEN_SECRET}" "$@"; }

is_dry_run() { [[ "${DRY_RUN}" == "true" || "${DRY_RUN}" == "1" ]]; }

# One transport blip used to abort the whole script under `set -e` and leave
# every later phase unrun, so reads retry until they come back as parseable
# JSON. A body that does not parse is treated as a failed attempt, not as data.
api_get_json() {
  local url="$1" label="$2" attempt body
  for attempt in $(seq 1 "${HTTP_ATTEMPTS}"); do
    if body="$(api --fail "${url}")" && jq empty <<<"${body}" >/dev/null 2>&1; then
      printf '%s' "${body}"
      return 0
    fi
    echo "  could not read ${label}, attempt ${attempt}/${HTTP_ATTEMPTS}..." >&2
    sleep "${RETRY_DELAY_SECONDS}"
  done
  echo "::error::Could not read ${label} from the API after ${HTTP_ATTEMPTS} attempts." >&2
  return 1
}

# No --fail, so -w reports the status instead of curl swallowing it. A 4xx is
# returned as a failure but never retried: the status will not change on its
# own, and inside a poll loop the next pass re-drives the request anyway.
mutate() {
  local attempt code

  if is_dry_run; then
    echo "  [dry-run] would send: $*"
    return 0
  fi

  for attempt in $(seq 1 "${HTTP_ATTEMPTS}"); do
    if code="$(api -o /dev/null -w '%{http_code}' "$@")"; then
      if [[ "${code}" -ge 400 ]]; then
        echo "  API returned HTTP ${code}."
        return 1
      fi
      return 0
    fi
    echo "  request failed at the transport layer, attempt ${attempt}/${HTTP_ATTEMPTS}..."
    sleep "${RETRY_DELAY_SECONDS}"
  done
  echo "  request failed after ${HTTP_ATTEMPTS} attempts."
  return 1
}

# jq exits non-zero on an unexpected document, but inside the process
# substitution that feeds mapfile that status is lost and the caller reads an
# empty array as "nothing to clean up". Check the envelope first so an error
# body can never be mistaken for an empty organization.
require_result_array() {
  local doc="$1" label="$2"
  if ! jq -e 'has("result") and (.result | type == "array")' <<<"${doc}" >/dev/null 2>&1; then
    echo "::error::Unexpected ${label} response; refusing to read it as empty." >&2
    head -c 500 <<<"${doc}" >&2
    echo >&2
    return 1
  fi
}

# Last resort at the deadline: a service wedged in `stopping` never reaches
# `stopped`, so the state machine below can never fire its DELETE. Try the
# DELETE anyway -- it takes if the backend has quietly finished, and a 4xx costs
# nothing -- then name everything still alive so a human can sweep by suffix.
report_stuck_services() {
  local entry id state name
  echo "::error::Gave up after ${SWEEP_TIMEOUT_SECONDS}s with $# service(s) alive for suffix ${SUFFIX}."
  for entry in "$@"; do
    IFS=$'\t' read -r id state name <<<"${entry}"
    echo "::error::  leaked service ${id} (${name}) in state ${state}"
    mutate -X DELETE "${ORG_URL}/services/${id}" || true
  done
}

# A service only accepts DELETE once it reports `stopped`, so this polls: stop
# whatever is running, delete whatever has stopped, repeat until nothing matches.
cleanup_services() {
  local deadline=$((SECONDS + SWEEP_TIMEOUT_SECONDS))
  local output entry id state name
  local -a entries

  echo "Deleting any service with suffix ${SUFFIX}..."

  while :; do
    output="$(api_get_json "${ORG_URL}/services" "services")" || return 1
    require_result_array "${output}" "services" || return 1
    mapfile -t entries < <(jq --arg suffix "${SUFFIX}" -r \
      '.result[] | select(.name | contains($suffix)) | [.id, .state, .name] | @tsv' <<<"${output}")

    if [[ "${#entries[@]}" -eq 0 ]]; then
      echo "No services to cleanup."
      return 0
    fi

    echo "There are ${#entries[@]} services to be cleaned up."

    if [[ "${SECONDS}" -ge "${deadline}" ]]; then
      report_stuck_services "${entries[@]}"
      return 1
    fi

    for entry in "${entries[@]}"; do
      IFS=$'\t' read -r id state name <<<"${entry}"

      case "${state}" in
      stopped)
        echo "Deleting service ${id}..."
        mutate -X DELETE "${ORG_URL}/services/${id}" || true
        ;;
      stopping)
        echo "Service ${id} is stopping, waiting..."
        ;;
      *)
        echo "Stopping service ${id}..."
        mutate -X PATCH "${ORG_URL}/services/${id}/state" \
          -H 'Content-Type: application/json' --data '{"command": "stop"}' || true
        ;;
      esac
    done

    if is_dry_run; then
      echo "[dry-run] stopping after one pass; nothing was changed."
      return 0
    fi

    echo "Waiting ${POLL_INTERVAL_SECONDS} seconds..."
    sleep "${POLL_INTERVAL_SECONDS}"
  done
}

# A failed apply skips terraform destroy, so Postgres services can leak here.
# Replicas sort first (isPrimary=false) because a primary cannot be deleted
# while a replica is attached to it.
cleanup_postgres() {
  local deadline=$((SECONDS + SWEEP_TIMEOUT_SECONDS))
  local output id
  local -a ids

  echo "Deleting Managed Postgres services with suffix ${SUFFIX}..."

  while :; do
    output="$(api_get_json "${ORG_URL}/postgres" "Managed Postgres services")" || return 1
    require_result_array "${output}" "Managed Postgres" || return 1
    mapfile -t ids < <(jq --arg suffix "${SUFFIX}" -r \
      '.result | sort_by(.isPrimary)[] | select(.name | contains($suffix)) | .id' <<<"${output}")

    if [[ "${#ids[@]}" -eq 0 ]]; then
      echo "No Managed Postgres services to cleanup."
      return 0
    fi

    echo "There are ${#ids[@]} Managed Postgres services to be cleaned up."

    if [[ "${SECONDS}" -ge "${deadline}" ]]; then
      echo "::error::Gave up after ${SWEEP_TIMEOUT_SECONDS}s with ${#ids[@]} Managed Postgres service(s) alive for suffix ${SUFFIX}: ${ids[*]}"
      return 1
    fi

    for id in "${ids[@]}"; do
      echo "Deleting Managed Postgres service ${id}..."
      mutate -X DELETE "${ORG_URL}/postgres/${id}" || true
    done

    if is_dry_run; then
      echo "[dry-run] stopping after one pass; nothing was changed."
      return 0
    fi

    echo "Waiting ${POLL_INTERVAL_SECONDS} seconds..."
    sleep "${POLL_INTERVAL_SECONDS}"
  done
}

cleanup_private_endpoints() {
  local output entry id cloud_provider region body rc=0
  local -a entries

  echo "Cleanup of private link endpoints under the terraform organization..."

  output="$(api_get_json "${ORG_URL}" "organization ${ORGANIZATION_ID}")" || return 1
  mapfile -t entries < <(jq -r \
    '(.result.privateEndpoints // [])[] | [.id, .cloudProvider, .region] | @tsv' <<<"${output}")

  if [[ "${#entries[@]}" -eq 0 ]]; then
    echo "No private endpoints to cleanup."
    return 0
  fi

  for entry in "${entries[@]}"; do
    IFS=$'\t' read -r id cloud_provider region <<<"${entry}"
    if [[ -z "$id" || -z "$cloud_provider" || -z "$region" ]]; then
      echo "::error::Missing required field(s) in private endpoint data: $entry" >&2
      echo "  ID: ${id:-<empty>}" >&2
      echo "  CLOUD_PROVIDER: ${cloud_provider:-<empty>}" >&2
      echo "  REGION: ${region:-<empty>}" >&2
      rc=1
      continue
    fi
    body=$(cat <<EOF
{
  "privateEndpoints": {
    "remove": [
      {
        "id": "$id",
        "cloudProvider": "$cloud_provider",
        "region": "$region"
      }
    ]
  }
}
EOF
)
    echo "Deleting endpoint..."
    echo "  ID: $id"
    echo "  CLOUD_PROVIDER: $cloud_provider"
    echo "  REGION: $region"
    mutate -X PATCH "${ORG_URL}" -H 'Content-Type: application/json' -d "$body" || {
      echo "::error::Could not remove private endpoint ${id}."
      rc=1
    }
  done

  return "${rc}"
}

cleanup_roles() {
  local output role_id rc=0
  local -a role_ids

  echo "Cleanup of custom roles with suffix ${SUFFIX}..."

  output="$(api_get_json "${ORG_URL}/roles" "roles")" || return 1
  require_result_array "${output}" "roles" || return 1
  mapfile -t role_ids < <(jq --arg suffix "${SUFFIX}" -r \
    '.result[] | select(.type == "custom" and (.name | contains($suffix))) | .id' <<<"${output}")

  if [[ "${#role_ids[@]}" -eq 0 ]]; then
    echo "No roles to cleanup."
    return 0
  fi

  echo "There are ${#role_ids[@]} roles to be cleaned up."
  for role_id in "${role_ids[@]}"; do
    echo "Deleting role ${role_id}..."
    mutate -X DELETE "${ORG_URL}/roles/${role_id}" || {
      echo "::error::Could not delete custom role ${role_id}."
      rc=1
    }
  done

  return "${rc}"
}

if is_dry_run; then
  echo "DRY RUN: listing what matches suffix ${SUFFIX} in organization ${ORGANIZATION_ID}; no changes will be made."
fi

FAILED_PHASES=()

# Every phase runs even if an earlier one failed. They clean up unrelated
# resources, and letting the first failure skip the rest is how a single curl
# error turns into a pile of leaked services, Postgres instances and roles.
run_phase() {
  local name="$1"
  shift
  if "$@"; then
    return 0
  fi
  echo "::error::Cleanup phase '${name}' failed for suffix ${SUFFIX}."
  FAILED_PHASES+=("${name}")
}

run_phase services cleanup_services
run_phase postgres cleanup_postgres
run_phase private-endpoints cleanup_private_endpoints
run_phase roles cleanup_roles

if [[ "${#FAILED_PHASES[@]}" -ne 0 ]]; then
  echo "::error::Cleanup incomplete for suffix ${SUFFIX}: ${FAILED_PHASES[*]}. Leftovers may still be running."
  exit 1
fi

echo "Cleanup complete."
