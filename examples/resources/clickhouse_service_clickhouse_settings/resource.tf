resource "clickhouse_service" "svc" {
  ...
}

resource "clickhouse_service_clickhouse_settings" "example" {
  service_id = clickhouse_service.svc.id

  settings = {
    compatibility  = "26.2"
    max_query_size = "262144"
  }
}

resource "clickhouse_service_clickhouse_settings" "read_only" {
  service_id     = clickhouse_service.svc.id
  merges_enabled = false
}
