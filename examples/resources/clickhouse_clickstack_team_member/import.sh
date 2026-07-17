# Team members can be imported by their email address.
terraform import clickhouse_clickstack_team_member.alice alice@example.com

# For a member in a non-default team (multi-team / EE deployments), prefix the
# email with the team ID:
terraform import clickhouse_clickstack_team_member.alice 65f0c0ffeecafef00dba5e01/alice@example.com
