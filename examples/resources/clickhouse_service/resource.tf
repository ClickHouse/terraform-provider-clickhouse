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

  tags = {
    Environment = "Staging",
  }

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
}

# Horizontal autoscaling: the replica count scales between min_replicas and max_replicas at a fixed
# per-replica memory (min_replica_memory_gb == max_replica_memory_gb). Requires horizontal autoscaling
# to be enabled for your organization. num_replicas is forbidden in horizontal mode.
resource "clickhouse_service" "horizontal_service" {
  name           = "My Horizontally-Scaling Service"
  cloud_provider = "aws"
  region         = "us-east-1"
  idle_scaling   = true

  ip_access = [
    {
      source      = "192.168.2.63"
      description = "Test IP"
    }
  ]

  autoscaling_mode      = "horizontal"
  min_replicas          = 3
  max_replicas          = 10
  min_replica_memory_gb = 16
  max_replica_memory_gb = 16

  idle_timeout_minutes = 5

  password_hash = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
}
