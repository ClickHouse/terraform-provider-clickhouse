# Alerts can be imported by their ID.
terraform import clickhouse_clickstack_alert.too_many_errors 507f1f77bcf86cd799439011

# For an alert in a non-default team (multi-team / EE deployments), prefix the ID
# with the team ID:
terraform import clickhouse_clickstack_alert.too_many_errors 65f0c0ffeecafef00dba5e01/507f1f77bcf86cd799439011

# To find the ID, list all alerts via the API. The response is an envelope of the
# form {"data": [{"id": "...", "name": "...", "savedSearchId": "...", ...}]}.
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/alerts" | jq -r '.data[] | "\(.id)\t\(.name)"'
