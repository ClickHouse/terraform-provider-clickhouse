variable "service_id" {
  description = "ID of an existing ClickHouse Cloud service."
  type        = string
}

resource "clickhouse_saved_query" "example" {
  service_id = var.service_id

  name     = "Example query"
  sql      = "SELECT 1 AS value"
  database = "default"
}
