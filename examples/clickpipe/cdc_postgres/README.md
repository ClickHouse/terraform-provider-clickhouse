## ClickPipe CDC with Postgres example

This example demonstrates how to deploy a Postgres CDC ClickPipe with a PostgreSQL database as the input source, including CDC infrastructure configuration for optimal performance.

### What this example creates

- A Postgres CDC ClickPipe that replicates data from PostgreSQL to ClickHouse
- CDC infrastructure resource for scaling the shared CDC compute resources
- Table mappings from Postgres source tables to ClickHouse destination tables

### Prerequisites

- A ClickHouse Cloud service
- A PostgreSQL database with:
  - Logical replication enabled (`wal_level = logical`)
  - A replication slot created (or specify `publication_name` to have ClickPipes create it)
  - Network access from ClickHouse Cloud to your PostgreSQL instance

### Using a Reverse Private Endpoint

For secure, private connectivity to your PostgreSQL instance, you can use a [reverse private endpoint](../reverse_private_endpoint_vpce_service). Create the RPE resource first, then use its DNS name as the `host` in your Postgres source configuration:

```hcl
resource "clickhouse_clickpipes_reverse_private_endpoint" "pg_endpoint" {
  service_id                = var.service_id
  description               = "RPE for Postgres CDC"
  type                      = "VPC_ENDPOINT_SERVICE"
  vpc_endpoint_service_name = var.vpc_endpoint_service_name
}

resource "clickhouse_clickpipe" "postgres_cdc" {
  name       = "Postgres CDC ClickPipe"
  service_id = var.service_id

  depends_on = [clickhouse_clickpipe_cdc_infrastructure.infra]

  source = {
    postgres = {
      host     = clickhouse_clickpipes_reverse_private_endpoint.pg_endpoint.dns_names[0]
      port     = var.postgres_port
      database = var.postgres_database
      # ... remaining configuration
    }
  }

  destination = {
    database = "default"
  }
}
```

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`

## Notes

- The CDC infrastructure resource is shared across all CDC ClickPipes in the service
- Only one CDC infrastructure resource should be created per service
- Table mappings define which Postgres tables to replicate and their destination in ClickHouse
- The `replication_mode` can be `cdc`, `snapshot`, or `cdc_only`
