resource "clickhouse_postgres_service" "example" {
  ...
}

resource "clickhouse_postgres_private_endpoint" "example" {
  postgres_id = clickhouse_postgres_service.example.id

  # Optional: allow VPC endpoints in other AWS regions to connect.
  supported_regions = ["us-west-2"]
}

resource "aws_vpc_endpoint" "postgres" {
  vpc_id             = aws_vpc.example.id
  service_name       = clickhouse_postgres_private_endpoint.example.endpoint_service_name
  vpc_endpoint_type  = "Interface"
  subnet_ids         = [aws_subnet.example.id]
  security_group_ids = [aws_security_group.postgres.id]

  # For a VPC endpoint in supported_regions, set service_region to the Postgres service's region.
  # service_region = "us-east-1"
}

resource "clickhouse_postgres_private_endpoint_attachment" "example" {
  postgres_id = clickhouse_postgres_private_endpoint.example.postgres_id
  allowed_vpc_endpoints = [
    { vpc_endpoint_id = aws_vpc_endpoint.postgres.id, description = "Application VPC" },
  ]
}

output "postgres_private_hostname" {
  value = clickhouse_postgres_private_endpoint.example.private_hostname
}
