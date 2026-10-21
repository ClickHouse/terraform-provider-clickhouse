# Source links are imported by the ID of the source they belong to. Every link
# the source holds is imported; remove any you do not want managed from config
# afterwards, or the next apply clears it.
terraform import clickhouse_clickstack_source_links.traces 507f1f77bcf86cd799439011

# For a source in a non-default team (multi-team / EE deployments), prefix the
# ID with the team ID:
terraform import clickhouse_clickstack_source_links.traces 65f0c0ffeecafef00dba5e01/507f1f77bcf86cd799439011
