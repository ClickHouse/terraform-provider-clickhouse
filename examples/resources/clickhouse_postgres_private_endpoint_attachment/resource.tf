resource "clickhouse_postgres_private_endpoint" "example" {
  ...
}

resource "clickhouse_postgres_private_endpoint_attachment" "example" {
  postgres_id = clickhouse_postgres_private_endpoint.example.postgres_id
  allowed_vpc_endpoints = [
    { vpc_endpoint_id = "vpce-0123456789abcdef0", description = "Application VPC" },
    { vpc_endpoint_id = "vpce-0123456789abcdef1" },
  ]
}
