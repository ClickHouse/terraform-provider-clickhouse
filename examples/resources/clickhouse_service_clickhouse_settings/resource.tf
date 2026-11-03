variable "service_id" {
  description = "ID of an existing ClickHouse Cloud service."
  type        = string
}

resource "clickhouse_service_clickhouse_settings" "example" {
  service_id = var.service_id

  settings = {
    max_query_size = "524288"
  }
}
