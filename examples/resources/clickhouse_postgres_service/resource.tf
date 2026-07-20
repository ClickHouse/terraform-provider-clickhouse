resource "clickhouse_postgres_service" "example" {
  name           = "my-postgres"
  cloud_provider = "aws"
  region         = "us-east-1"
  size           = "m6gd.large"

  # High-availability mode — number of standby replicas:
  #   "none"  – primary only, no standby (default)
  #   "async" – 1 standby, asynchronous replication
  #   "sync"  – 2 standbys, synchronous replication
  # See https://clickhouse.com/docs/cloud/managed-postgres/high-availability
  ha_type = "async"

  tags = {
    environment = "production"
    team        = "data"
  }

  # A standard service must declare a credential: `password` (stored in
  # sensitive state) or `password_wo` + `password_wo_version` (write-only,
  # never stored in state).
  password = "Example123Secret"
}
