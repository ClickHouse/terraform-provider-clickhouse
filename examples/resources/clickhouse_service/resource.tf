resource "clickhouse_service" "service" {
  name           = "My Service"
  cloud_provider = "aws"
  region         = "us-east-1"
  idle_scaling   = true

  ip_access = [
    {
      source      = "192.168.2.63"
      description = "Test IP"
    }
  ]

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
}
