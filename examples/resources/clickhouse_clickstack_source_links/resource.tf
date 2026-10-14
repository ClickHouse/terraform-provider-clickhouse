# Logs and traces that link to each other. Each link needs the other source's
# ID, so one direction is set on the source itself and the other here, after
# both sources exist.
resource "clickhouse_clickstack_source" "logs" {
  name          = "Logs"
  kind          = "log"
  connection_id = clickhouse_clickstack_connection.main.id

  from = {
    database_name = "otel"
    table_name    = "otel_logs"
  }

  timestamp_value_expression      = "Timestamp"
  default_table_select_expression = "Timestamp, ServiceName, SeverityText, Body"

  trace_source_id = clickhouse_clickstack_source.traces.id
}

resource "clickhouse_clickstack_source" "traces" {
  name          = "Traces"
  kind          = "trace"
  connection_id = clickhouse_clickstack_connection.main.id

  from = {
    database_name = "otel"
    table_name    = "otel_traces"
  }

  timestamp_value_expression      = "Timestamp"
  default_table_select_expression = "Timestamp, SpanName, ServiceName, Duration"

  duration_expression       = "Duration"
  duration_precision        = 9
  trace_id_expression       = "TraceId"
  span_id_expression        = "SpanId"
  parent_span_id_expression = "ParentSpanId"
  span_name_expression      = "SpanName"
  span_kind_expression      = "SpanKind"

  # log_source_id is left unset: clickhouse_clickstack_source_links.traces owns it.
}

resource "clickhouse_clickstack_source_links" "traces" {
  source_id     = clickhouse_clickstack_source.traces.id
  log_source_id = clickhouse_clickstack_source.logs.id
}
