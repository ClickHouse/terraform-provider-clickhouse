variable "service_id" {
  description = "ID of an existing ClickHouse Cloud service."
  type        = string
}

data "clickhouse_api_key_id" "current" {}

resource "clickhouse_query_api_endpoint" "format_date" {
  service_id = var.service_id

  name     = "format-date"
  sql      = "SELECT formatDateTime({date:Date}, '%F') AS date"
  database = "default"
  parameters = {
    date = "2026-08-13"
  }

  api_key_ids     = [data.clickhouse_api_key_id.current.id]
  roles           = ["sql_console_read_only"]
  allowed_origins = ["https://example.com"]
}

output "query_api_endpoint_url" {
  description = "Public URL of the Query API endpoint."
  value       = clickhouse_query_api_endpoint.format_date.url
}
