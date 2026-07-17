# This Terraform resource maps directly to the HyperDX v2 API dashboard body.
# You can obtain an existing dashboard's JSON from GET /api/v2/dashboards/{id},
# or let Terraform import it with: terraform import clickhouse_clickstack_dashboard.<name> <id>
# Note: PromQL tiles are not supported by the v2 API and cannot be managed here.

resource "clickhouse_clickstack_dashboard" "collectors" {
  dashboard_json = jsonencode({
    name = "OTel Collectors"
    tags = ["otel"]
    tiles = [
      {
        name = "Exporter sent spans"
        x    = 0, y = 0, w = 6, h = 3
        config = {
          displayType   = "line"
          sourceId      = var.metrics_source_id
          where         = "ServiceName:clickstack"
          whereLanguage = "lucene"
          groupBy       = "ServiceName"
          select = [{
            aggFn           = "sum"
            valueExpression = "Value"
            metricType      = "sum"
            metricName      = "otelcol_exporter_sent_spans_total"
            alias           = "Sent spans"
          }]
        }
      },
      {
        name = "Top services by log volume"
        x    = 6, y = 0, w = 6, h = 3
        config = {
          configType   = "sql"
          displayType  = "table"
          connectionId = var.connection_id
          sqlTemplate  = "SELECT ServiceName, count() AS logs FROM otel_logs GROUP BY ServiceName ORDER BY logs DESC LIMIT 20"
        }
      }
    ]
  })
}

variable "metrics_source_id" { type = string }
variable "connection_id" { type = string }
