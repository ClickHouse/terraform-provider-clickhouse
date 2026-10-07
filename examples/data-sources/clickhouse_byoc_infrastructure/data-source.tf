variable "byoc_id" {
  description = "ID of an existing BYOC infrastructure (e.g. created from the ClickHouse Cloud console)."
  type        = string
}

data "clickhouse_byoc_infrastructure" "this" {
  id = var.byoc_id
}

# Deploy a service into the existing BYOC infrastructure.
resource "clickhouse_service" "byoc" {
  name           = "byoc service"
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
  cloud_provider = data.clickhouse_byoc_infrastructure.this.cloud_provider
  region         = data.clickhouse_byoc_infrastructure.this.region_id
  byoc_id        = data.clickhouse_byoc_infrastructure.this.id

  # BYOC services use custom instance profiles; discover the available
  # profiles with the clickhouse_service_profiles data source.
  profile               = "v1-standard-byoc-4"
  min_replica_memory_gb = 8 # must equal the profile's memory_gi
  max_replica_memory_gb = 8
}

output "byoc_state" {
  value = data.clickhouse_byoc_infrastructure.this.state
}
