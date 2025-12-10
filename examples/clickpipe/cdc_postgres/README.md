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

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`

## Notes

- The CDC infrastructure resource is shared across all CDC ClickPipes in the service
- Only one CDC infrastructure resource should be created per service
- Table mappings define which Postgres tables to replicate and their destination in ClickHouse
- The `replication_mode` can be `cdc`, `snapshot`, or `cdc_only`
