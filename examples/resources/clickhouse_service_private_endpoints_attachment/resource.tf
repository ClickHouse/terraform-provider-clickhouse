resource "clickhouse_service" "svc" {
  ...
}

resource "clickhouse_private_endpoint_registration" "endpoint" {
  ...
}

resource "clickhouse_service_private_endpoints_attachment" "attachment" {
  private_endpoint_ids = [
    clickhouse_private_endpoint_registration.endpoint.id
  ]
  service_id = clickhouse_service.svc.id
}
