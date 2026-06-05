~> **Note:** This data source is in alpha and its behavior may change in future provider versions.

Fetches the PEM-encoded CA certificate chain for a
[ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
service. Use it to pin the CA when connecting with `sslmode=verify-full`.

Input `service_id`; output `certificate` (the PEM chain).

## Example

```hcl
data "clickhouse_postgres_service_ca_certificates" "example" {
  service_id = clickhouse_postgres_service.example.id
}

resource "local_file" "ca" {
  content  = data.clickhouse_postgres_service_ca_certificates.example.certificate
  filename = "${path.module}/pg-ca.pem"
}
```
