#!/usr/bin/env bash

set -euo pipefail

API_URL="${API_URL:-"https://api.clickhouse.cloud/v1"}"
ORGANIZATION_ID="${ORGANIZATION_ID:?"ORGANIZATION_ID cannot be empty"}"
TOKEN_KEY="${TOKEN_KEY:?"TOKEN_KEY cannot be empty"}"
TOKEN_SECRET="${TOKEN_SECRET:?"TOKEN_SECRET cannot be empty"}"
SUFFIX="${SUFFIX:?"SUFFIX cannot be empty"}"

echo "Deleting any service with suffix ${SUFFIX}..."

while :; do
  OUTPUT="$(curl -su "${TOKEN_KEY}:${TOKEN_SECRET}" "${API_URL}/organizations/${ORGANIZATION_ID}/services")"
  mapfile -t IDS < <(jq --arg suffix "${SUFFIX}" -r '.result[] | select(.name | contains($suffix)) | (.id + "," + .state)' <<<"${OUTPUT}")

  if [[ "${#IDS[@]}" -eq 0 ]]; then
    echo "No services to cleanup."
    break
  fi

  echo "There are ${#IDS[@]} services to be cleaned up."

  for ID_AND_STATUS in "${IDS[@]}"; do
    ID="$(echo "${ID_AND_STATUS}" | cut -d"," -f1)"
    STATUS="$(echo "${ID_AND_STATUS}" | cut -d"," -f2)"

    case "${STATUS}" in
    stopped)
      echo "Deleting service ${ID}..."
      curl -su "${TOKEN_KEY}:${TOKEN_SECRET}" -XDELETE "${API_URL}/organizations/${ORGANIZATION_ID}/services/${ID}" -o /dev/null
      ;;
    stopping)
      echo "Service ${ID} is stopping, waiting..."
      ;;
    *)
      echo "Stopping service ${ID}..."
      curl -su "${TOKEN_KEY}:${TOKEN_SECRET}" -XPATCH "${API_URL}/organizations/${ORGANIZATION_ID}/services/${ID}/state" --data '{"command": "stop"}' -H 'Content-Type: application/json' -o /dev/null
      ;;
    esac
  done

  echo "Waiting 5 seconds..."
  sleep 5
done

echo "Cleanup complete."
