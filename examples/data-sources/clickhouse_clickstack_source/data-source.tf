# Look up a source that ClickHouse Cloud created for the service, so dashboards
# and saved searches can reference it without hardcoding the id.
data "clickhouse_clickstack_source" "traces" {
  name = "Traces"
  kind = "trace"
}

output "traces_source_id" {
  value = data.clickhouse_clickstack_source.traces.id
}
