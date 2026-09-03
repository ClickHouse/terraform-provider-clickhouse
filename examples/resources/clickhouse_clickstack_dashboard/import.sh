# Dashboards can be imported by their ID.
terraform import clickhouse_clickstack_dashboard.collectors 65f0c0ffeecafef00dba5e02

# For a dashboard in a non-default team (multi-team / EE deployments), prefix the
# ID with the team ID:
terraform import clickhouse_clickstack_dashboard.collectors 65f0c0ffeecafef00dba5e01/65f0c0ffeecafef00dba5e02

# `terraform plan -generate-config-out=...` writes the dashboard body with the
# literal ids the API returns (sourceId on every tile, for instance). Terraform
# generates config from state alone and cannot know those ids belong to other
# resources, so replace them by hand to link the dashboard to its sources:
# sourceId = clickhouse_clickstack_source.traces.id

# Tile alerts are separate resources and are NOT imported with the dashboard.
# Import each one as a clickhouse_clickstack_alert (source = "tile") by its own
# alert ID; see the clickhouse_clickstack_alert import example for how to list them.
