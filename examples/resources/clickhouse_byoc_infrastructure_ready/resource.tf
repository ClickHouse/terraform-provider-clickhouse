# Wait for a restricted-access BYOC infrastructure to become ready once its
# connector is installed, then deploy services into it.
resource "clickhouse_byoc_infrastructure_ready" "restricted" {
  byoc_id    = clickhouse_byoc_infrastructure.restricted.id
  depends_on = [helm_release.connector]
}

resource "clickhouse_service" "byoc" {
  name           = "restricted byoc service"
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
  cloud_provider = "aws"
  region         = clickhouse_byoc_infrastructure.restricted.region_id
  byoc_id        = clickhouse_byoc_infrastructure_ready.restricted.id

  profile               = "v1-standard-byoc-4"
  min_replica_memory_gb = 8
  max_replica_memory_gb = 8
}
