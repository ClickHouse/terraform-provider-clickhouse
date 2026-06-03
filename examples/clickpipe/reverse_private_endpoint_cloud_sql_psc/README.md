## Cloud SQL PSC reverse private endpoint example

This example creates a ClickPipes reverse private endpoint for an existing Google Cloud SQL Private Service Connect service attachment and assigns a custom private DNS name for ClickPipes to use as the database host.

### What this example creates

- A `GCP_PSC_SERVICE_ATTACHMENT` ClickPipes reverse private endpoint
- A custom private DNS mapping for the Cloud SQL hostname

### What this example does not create

- Cloud SQL instances
- Private Service Connect service attachments
- ClickPipes
- ClickHouse services

### Prerequisites

- A ClickHouse Cloud service
- An existing Cloud SQL instance with Private Service Connect configured
- The Cloud SQL PSC service attachment URI in this format: `projects/{project}/regions/{region}/serviceAttachments/{name}`
- A private DNS name to use from ClickPipes, for example `cloudsql-postgres.internal.example.com`

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`.
- Run `terraform <plan|apply> -var-file=variables.tfvars`.

## Use with a PostgreSQL CDC ClickPipe

After this reverse private endpoint is ready, use the custom DNS name as the PostgreSQL source host:

```hcl
resource "clickhouse_clickpipe" "postgres_cdc" {
  name       = "Cloud SQL Postgres CDC ClickPipe"
  service_id = var.service_id

  source = {
    postgres = {
      host     = clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns.cloud_sql.mapping[0].private_dns_name
      port     = 5432
      database = var.postgres_database

      credentials = {
        username = var.postgres_username
        password = var.postgres_password
      }

      settings = {
        replication_mode = "cdc"
      }

      table_mappings = [
        {
          source_schema_name = "public"
          source_table       = "users"
          target_table       = "users"
        }
      ]
    }
  }

  destination = {
    database = "default"
  }
}
```

## Use with a MySQL CDC ClickPipe

For Cloud SQL MySQL, use the same custom DNS name with port `3306`:

```hcl
resource "clickhouse_clickpipe" "mysql_cdc" {
  name       = "Cloud SQL MySQL CDC ClickPipe"
  service_id = var.service_id

  source = {
    mysql = {
      host = clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns.cloud_sql.mapping[0].private_dns_name
      port = 3306

      credentials = {
        username = var.mysql_username
        password = var.mysql_password
      }

      settings = {
        replication_mode      = "cdc"
        replication_mechanism = "GTID"
      }

      table_mappings = [
        {
          source_schema_name = var.mysql_database
          source_table       = "users"
          target_table       = "users"
        }
      ]
    }
  }

  destination = {
    database = "default"
  }
}
```
