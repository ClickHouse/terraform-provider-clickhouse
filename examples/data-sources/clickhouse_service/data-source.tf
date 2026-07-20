data "clickhouse_service" "example" {
  id = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
}

output "service_endpoints" {
  value = data.clickhouse_service.example.endpoints
}
