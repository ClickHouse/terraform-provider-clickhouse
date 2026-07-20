~> **Note:** This data source is in alpha and its behavior may change in future provider versions.

Fetches a single [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
service by ID, including its current `pg_config` / `pgbouncer_config`.

Returns the service's server-reported attributes: `cloud_provider`, `region`,
`size`, `ha_type`, `postgres_version`, status (`state`, `created_at`,
`is_primary`), connectivity (`hostname`, `port`, `username`), and
`tags` / `pg_config` / `pgbouncer_config` (`port` is the fixed default 5432 —
the server doesn't expose a per-instance port). The API returns no credentials:
there is no password field and no connection string. Compose a connection URI
from `hostname`, `port`, and `username`, plus the password declared on the
managing `clickhouse_postgres_service` resource. The write-time create
inputs — `read_replica_of`, `restore_to_point_in_time` — are not
read back. `tags`, `pg_config`, and `pgbouncer_config` are read-only string maps.

## Example

```hcl
data "clickhouse_postgres_service" "example" {
  id = "5f1a3d9d-3d8e-8ed0-8d25-545aec9d5e3b"
}

output "host" {
  value = data.clickhouse_postgres_service.example.hostname
}
```
