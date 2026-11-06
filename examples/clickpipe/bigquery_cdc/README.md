## ClickPipe BigQuery CDC example

This example demonstrates how to deploy a BigQuery CDC ClickPipe using Terraform.
With `replication_mode = "cdc"` the pipe first takes an initial snapshot of each table and then keeps replicating changes.

It provisions all necessary GCP prerequisites, including:
- GCS staging bucket
- IAM service account with required permissions
- IAM service account key

BigQuery dataset and tables must already exist. With the default `query_based` replication method every source table needs a `TIMESTAMP` column (set with `bigquery_watermark_column`) that is updated whenever a row changes.

## Variations

- Skip the initial snapshot and only replicate new changes: set `replication_mode = "cdc_only"`.
- Take a one-time snapshot without CDC: set `replication_mode = "snapshot"` and remove `replication_method`, `sync_interval_seconds` and `query_cdc_watermark_column`.
- Read changes from the BigQuery change history instead of polling a watermark column: set `replication_method = "events_based"`, drop `query_cdc_watermark_column` and set `events_function = "appends"` or `"changes"` on every table mapping.

Settings of an existing BigQuery pipe cannot be changed in place, changing them recreates the pipe.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
