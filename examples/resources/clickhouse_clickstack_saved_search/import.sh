# Saved searches can be imported by their ID.
terraform import clickhouse_clickstack_saved_search.errors 507f1f77bcf86cd799439011

# For a saved search in a non-default team (multi-team / EE deployments), prefix
# the ID with the team ID:
terraform import clickhouse_clickstack_saved_search.errors 65f0c0ffeecafef00dba5e01/507f1f77bcf86cd799439011

# To find the ID, list all saved searches via the API. The response is an
# envelope of the form {"data": [{"id": "...", "name": "...", ...}]}.
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/saved-searches" | jq -r '.data[] | "\(.id)\t\(.name)"'
