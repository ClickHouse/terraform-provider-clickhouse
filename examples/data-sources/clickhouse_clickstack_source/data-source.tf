# Look up the sources ClickHouse Cloud created for the service, so dashboards
# and saved searches can reference them without hardcoding ids.
data "clickhouse_clickstack_source" "default" {
  for_each = {
    logs     = { name = "Logs", kind = "log" }
    traces   = { name = "Traces", kind = "trace" }
    metrics  = { name = "Metrics", kind = "metric" }
    sessions = { name = "Sessions", kind = "session" }
  }

  name = each.value.name
  kind = each.value.kind
}

output "source_ids" {
  value = { for k, s in data.clickhouse_clickstack_source.default : k => s.id }
}
