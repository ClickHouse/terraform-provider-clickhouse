# Alerts can be imported by their ID.
terraform import clickhouse_clickstack_alert.too_many_errors 507f1f77bcf86cd799439011

# For an alert in a non-default team (multi-team / EE deployments), prefix the ID
# with the team ID:
terraform import clickhouse_clickstack_alert.too_many_errors 65f0c0ffeecafef00dba5e01/507f1f77bcf86cd799439011

# To find the ID, list all alerts via the API. The response is an envelope of the
# form {"data": [{"id": "...", "name": "...", "savedSearchId": "...", ...}]}.
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/alerts" | jq -r '.data[] | "\(.id)\t\(.name)"'

# Tile alerts (source = "tile") are imported the same way, by the alert's own ID.
# Importing a dashboard does not import its tile alerts: terraform import maps one
# ID to one resource. List the tile alerts with their dashboard and tile ids:
curl -s -H "Authorization: Bearer $CLICKSTACK_API_KEY" \
  "$CLICKSTACK_ENDPOINT/api/v2/alerts" \
  | jq -r '.data[] | select(.source == "tile") | "\(.id)\t\(.dashboardId)\t\(.tileId)\t\(.name)"'

# `terraform plan -generate-config-out=...` writes the alert with literal ids for
# dashboard_id, tile_id and channel.webhook_id. Terraform generates config from
# state alone and cannot know those ids belong to other resources, so replace
# them by hand to link the alert to its dashboard tile and webhook:
#   dashboard_id = clickhouse_clickstack_dashboard.latency.id
#   tile_id      = clickhouse_clickstack_dashboard.latency.tile_ids["p95 latency"]
#   webhook_id   = clickhouse_clickstack_webhook.slack.id
# The generated `= null` lines can be deleted.
