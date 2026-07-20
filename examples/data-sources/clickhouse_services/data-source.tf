# All services in the organization
data "clickhouse_services" "all" {}

# Only services tagged Environment=production
data "clickhouse_services" "prod" {
  tags = {
    Environment = "production"
  }
}

output "prod_service_names" {
  value = [for s in data.clickhouse_services.prod.services : s.name]
}
