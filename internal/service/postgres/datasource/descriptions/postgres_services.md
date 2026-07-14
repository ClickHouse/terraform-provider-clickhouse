~> **Note:** This data source is in alpha and its behavior may change in future provider versions.

Lists all [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
services in the organization. Returns a `services` list of summary objects
(`id`, `name`, `cloud_provider`, `region`, `postgres_version`, `size`,
`ha_type`, `state`, `created_at`, `is_primary`).

The list endpoint does not return `connection_string`, `password`, or
`pg_config`; use the `clickhouse_postgres_service` data source (by ID) for the
full set of attributes.

## Example

```hcl
data "clickhouse_postgres_services" "all" {}

output "service_names" {
  value = [for s in data.clickhouse_postgres_services.all.services : s.name]
}
```
