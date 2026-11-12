# This Terraform resource maps directly to the ClickStack v2 API dashboard body.
# You can obtain an existing dashboard's JSON from GET /api/v2/dashboards/{id},
# or let Terraform import it with: terraform import clickhouse_clickstack_dashboard.<name> <id>

resource "clickhouse_clickstack_dashboard" "collectors" {
  dashboard_json = jsonencode({
    name = "OTel Collectors"
    tags = ["otel"]
    # PROMETHEUS_LABEL filters list a label's values from a promql source.
    # Tiles reference the selection as $variableName.
    filters = [
      {
        type         = "PROMETHEUS_LABEL"
        name         = "Job"
        sourceId     = var.promql_source_id
        label        = "job"
        variableName = "job"
      }
    ]
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
      },
      {
        name = "Series per job"
        x    = 0, y = 3, w = 6, h = 3
        config = {
          configType     = "promql"
          displayType    = "line"
          sourceId       = var.promql_source_id
          expressions    = [{ expression = "count by (job) ({__name__=~\".+\", job=~\"$job\"})" }]
          legendTemplate = "{{job}}"
        }
      }
    ]
  })
}

variable "metrics_source_id" { type = string }
variable "connection_id" { type = string }
variable "promql_source_id" { type = string }
