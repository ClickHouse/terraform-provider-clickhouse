# Wait for a restricted-access BYOC infrastructure to become ready once its
# connector is installed, then deploy services into it.
resource "clickhouse_byoc_infrastructure_ready" "restricted" {
  byoc_id    = clickhouse_byoc_infrastructure.restricted.id
  depends_on = [helm_release.connector]
}

# A service on restricted-access infrastructure runs at one fixed size: no
# profile, equal memory bounds, a fixed replica count and no idle scaling.
resource "clickhouse_service" "byoc" {
  name           = "restricted byoc service"
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
  cloud_provider = "aws"
  region         = clickhouse_byoc_infrastructure.restricted.region_id
  byoc_id        = clickhouse_byoc_infrastructure_ready.restricted.id

  ip_access = [
    {
      source      = "0.0.0.0/0"
      description = "Anywhere"
    }
  ]

  min_replica_memory_gb = 48
  max_replica_memory_gb = 48
  num_replicas          = 3
  idle_scaling          = false
}
