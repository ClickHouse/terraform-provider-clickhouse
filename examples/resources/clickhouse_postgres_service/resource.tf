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

  ip_access = [
    { source = "203.0.113.0/24", port_range = "5432", description = "Office Postgres" },
    { source = "203.0.113.0/24", port_range = "6432", description = "Office PgBouncer" },
  ]

  # A standard service must declare a credential: `password` (stored in
  # sensitive state) or `password_wo` + `password_wo_version` (write-only,
  # never stored in state).
  password = "Example123Secret"
}
