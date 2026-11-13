resource "clickhouse_clickstack_connection" "main" {
  name     = "Production ClickHouse"
  host     = "https://clickhouse.example.com:8443"
  username = "default"
  password = var.clickhouse_password
}

# A log source.
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

  service_name_expression        = "ServiceName"
  severity_text_expression       = "SeverityText"
  severity_number_expression     = "SeverityNumber"
  body_expression                = "Body"
  event_name_expression          = "EventName"
  resource_attributes_expression = "ResourceAttributes"
  event_attributes_expression    = "LogAttributes"
}

# A trace source correlated with the logs above.
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

  span_events_value_expression = "Events"
  span_links_value_expression  = "Links"

  log_source_id = clickhouse_clickstack_source.logs.id
}

# A metric source. Metric sources locate tables via metric_tables rather than
# from.table_name.
resource "clickhouse_clickstack_source" "metrics" {
  name          = "Metrics"
  kind          = "metric"
  connection_id = clickhouse_clickstack_connection.main.id

  from = {
    database_name = "otel"
  }

  timestamp_value_expression     = "TimeUnix"
  resource_attributes_expression = "ResourceAttributes"
  min_auto_granularity           = "1 minute"

  metric_tables = {
    gauge     = "otel_metrics_gauge"
    sum       = "otel_metrics_sum"
    histogram = "otel_metrics_histogram"
  }
}

# A PromQL source reading a ClickHouse TimeSeries table. PromQL is a preview
# feature and must be enabled on the ClickStack deployment.
resource "clickhouse_clickstack_source" "promql" {
  name          = "PromQL"
  kind          = "promql"
  connection_id = clickhouse_clickstack_connection.main.id

  from = {
    database_name = "default"
    table_name    = "metrics_ts"
  }

  # Required by the API but unused for promql.
  timestamp_value_expression = "timestamp"

  min_auto_granularity       = "1 minute"
}

# A PromQL source querying a Prometheus server. from is unused here, but the
# API requires non-empty values.
resource "clickhouse_clickstack_connection" "prometheus" {
  name                   = "Prometheus"
  host                   = "http://prometheus:9090"
  username               = ""
  is_prometheus_endpoint = true
}

resource "clickhouse_clickstack_source" "prometheus" {
  name          = "Prometheus"
  kind          = "promql"
  connection_id = clickhouse_clickstack_connection.prometheus.id

  from = {
    database_name = "default"
    table_name    = "default"
  }

  timestamp_value_expression = "timestamp"
}
