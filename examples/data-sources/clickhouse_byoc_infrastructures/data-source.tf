# List all BYOC infrastructures of the organization.
data "clickhouse_byoc_infrastructures" "all" {}

output "byoc_infrastructures" {
  value = data.clickhouse_byoc_infrastructures.all.infrastructures
}

# Look an infrastructure up by display name instead of hardcoding its ID.
locals {
  prod_byoc = one([
    for infra in data.clickhouse_byoc_infrastructures.all.infrastructures :
    infra if infra.display_name == "Production BYOC"
  ])
}

resource "clickhouse_service" "byoc" {
  name           = "byoc service"
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
  cloud_provider = local.prod_byoc.cloud_provider
  region         = local.prod_byoc.region_id
  byoc_id        = local.prod_byoc.id

  # BYOC services use custom instance profiles; discover the available
  # profiles with the clickhouse_service_profiles data source.
  profile               = "v1-standard-byoc-4"
  min_replica_memory_gb = 8 # must equal the profile's memory_gi
  max_replica_memory_gb = 8
}
