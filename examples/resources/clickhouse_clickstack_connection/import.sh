# Connections can be imported by their ID.
terraform import clickhouse_clickstack_connection.main 507f1f77bcf86cd799439012

# For a connection in a non-default team (multi-team / EE deployments), prefix
# the ID with the team ID so the import can resolve the team-scoped connection:
terraform import clickhouse_clickstack_connection.main 65f0c0ffeecafef00dba5e01/507f1f77bcf86cd799439012

# To find the ID, list all connections via the API. The response is an
# envelope of the form {"data": [{"id": "...", "name": "...", ...}]}.
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/connections"

# Pipe through jq to print just the id and name for each connection.
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/connections" | jq -r '.data[] | "\(.id)\t\(.name)"'
