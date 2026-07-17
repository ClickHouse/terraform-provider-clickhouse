# Dashboards can be imported by their ID.
terraform import clickhouse_clickstack_dashboard.collectors 65f0c0ffeecafef00dba5e02

# For a dashboard in a non-default team (multi-team / EE deployments), prefix the
# ID with the team ID:
terraform import clickhouse_clickstack_dashboard.collectors 65f0c0ffeecafef00dba5e01/65f0c0ffeecafef00dba5e02
