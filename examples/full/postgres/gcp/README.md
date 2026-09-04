## GCP Managed Postgres example

The Terraform code deploys following resources:
- 1 ClickHouse Managed Postgres service (primary) on GCP
- 1 read replica of the primary
- 3 data sources reading the primary back (by ID, all services, CA certificates)

NOTE: `clickhouse_postgres_service` is a beta resource — it ships in the regular provider build but its behavior may change in future provider versions.

Postgres on GCP is in **private preview**. Contact ClickHouse support to enable
access for your organization and selected region before running this example.
The default region is `us-west1` and the instance size is `c4a-highmem-4`;
size and region availability are validated by the API.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
