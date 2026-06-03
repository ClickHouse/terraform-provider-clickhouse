resource "clickhouse_service" "svc" {
  ...
}

resource "clickhouse_service_upgrade_window" "example" {
  service_id     = clickhouse_service.svc.id
  weekday        = 3 # Wednesday
  start_hour_utc = 12
}
