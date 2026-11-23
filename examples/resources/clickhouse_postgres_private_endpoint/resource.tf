resource "clickhouse_postgres_service" "example" {
  ...
}

resource "clickhouse_postgres_private_endpoint" "example" {
  postgres_id = clickhouse_postgres_service.example.id
}

resource "aws_vpc_endpoint" "postgres" {
  vpc_id             = aws_vpc.example.id
  service_name       = clickhouse_postgres_private_endpoint.example.endpoint_service_name
  vpc_endpoint_type  = "Interface"
  subnet_ids         = [aws_subnet.example.id]
  security_group_ids = [aws_security_group.postgres.id]
}

resource "clickhouse_postgres_private_endpoint_attachment" "example" {
  postgres_id = clickhouse_postgres_private_endpoint.example.postgres_id
  allowed_vpc_endpoints = {
    (aws_vpc_endpoint.postgres.id) = { description = "Application VPC" }
  }
}

output "postgres_private_hostname" {
  value = clickhouse_postgres_private_endpoint.example.private_hostname
}
