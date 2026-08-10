# Follow new UDF versions as they are published.
resource "clickhouse_udf_attachment" "development" {
  function_name = clickhouse_udf.echo_string.function_name
  service_id    = var.development_service_id
  version       = clickhouse_udf.echo_string.version
}

# Pin production to a chosen version.
resource "clickhouse_udf_attachment" "production" {
  function_name = clickhouse_udf.echo_string.function_name
  service_id    = var.production_service_id
  version       = var.echo_string_production_version
}
