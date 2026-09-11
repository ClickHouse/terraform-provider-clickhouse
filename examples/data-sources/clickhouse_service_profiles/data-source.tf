# Custom instance profiles available in a region
data "clickhouse_service_profiles" "us_east_1" {
  region_id = "us-east-1"
}

# Include dynamic BYOC profiles configured for a BYOC infrastructure
data "clickhouse_service_profiles" "byoc" {
  region_id = "us-east-1"
  byoc_id   = var.byoc_id
}

output "available_profiles" {
  value = data.clickhouse_service_profiles.byoc.profiles
}

# Use a discovered profile to create a service
resource "clickhouse_service" "byoc" {
  name           = "byoc-service"
  cloud_provider = "aws"
  region         = "us-east-1"
  byoc_id        = var.byoc_id

  profile               = data.clickhouse_service_profiles.byoc.profiles[0].profile
  min_replica_memory_gb = data.clickhouse_service_profiles.byoc.profiles[0].memory_gi
  max_replica_memory_gb = data.clickhouse_service_profiles.byoc.profiles[0].memory_gi
}
