# Sources can be imported by their ID.
terraform import clickhouse_clickstack_source.logs 507f1f77bcf86cd799439011

# For a source in a non-default team (multi-team / EE deployments), prefix the
# ID with the team ID so the import can resolve the team-scoped source:
terraform import clickhouse_clickstack_source.logs 65f0c0ffeecafef00dba5e01/507f1f77bcf86cd799439011

# To find the ID, list all sources via the API. The response is an envelope of
# the form {"data": [{"id": "...", "name": "...", "kind": "...", ...}]}.
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/sources" | jq -r '.data[] | "\(.id)\t\(.kind)\t\(.name)"'
