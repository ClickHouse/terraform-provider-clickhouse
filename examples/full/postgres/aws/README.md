## AWS Managed Postgres example

The Terraform code deploys following resources:
- 1 ClickHouse Managed Postgres service (primary) on AWS
- 1 read replica of the primary
- 3 data sources reading the primary back (by ID, all services, CA certificates)

NOTE: `clickhouse_postgres_service` is an alpha resource — it ships in the regular provider build but its behavior may change in future provider versions.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
