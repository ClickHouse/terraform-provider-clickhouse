# Webhooks can be imported by their ID.
terraform import clickhouse_clickstack_webhook.slack 507f1f77bcf86cd799439011

# For a webhook in a non-default team (multi-team / EE deployments), prefix the
# ID with the team ID so the import can resolve the team-scoped webhook:
terraform import clickhouse_clickstack_webhook.slack 65f0c0ffeecafef00dba5e01/507f1f77bcf86cd799439011

# Write-only secrets (headers, query_params) are never returned by the API, so
# they are null after import. Set them in config and bump the matching *_version
# to send them on the next apply.

# To find the ID, list all webhooks via the API. The response is an envelope of
# the form {"data": [{"id": "...", "name": "...", "service": "...", ...}]}.
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/webhooks" | jq -r '.data[] | "\(.id)\t\(.service)\t\(.name)"'
