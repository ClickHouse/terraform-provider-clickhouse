~> **Note:** This data source is in alpha and its behavior may change in future provider versions.

Fetches a single [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
service by ID, including its current `pg_config` / `pgbouncer_config`.

Returns the service's server-reported state: geometry (`cloud_provider`,
`region`, `size`, `ha_type`, `postgres_version`), status (`state`,
`created_at`, `is_primary`), connectivity (`hostname`, `username`,
`connection_string`), and `tags` / `pg_config` / `pgbouncer_config` (`port` is
the fixed default 5432 — the server doesn't expose a per-instance port). It does
**not** expose the resource's create/management inputs — `password`,
`password_wo`, `read_replica_of`, `restore_to_point_in_time` — since those are
write-time inputs, not read-back state. The `connection_string` (which embeds
the password) is returned and marked sensitive. `tags`, `pg_config`, and
`pgbouncer_config` are read-only string maps.

## Example

```hcl
data "clickhouse_postgres_service" "example" {
  id = "5f1a3d9d-3d8e-8ed0-8d25-545aec9d5e3b"
}

output "host" {
  value = data.clickhouse_postgres_service.example.hostname
}
```
