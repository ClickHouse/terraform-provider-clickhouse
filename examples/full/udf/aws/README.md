## AWS UDF example

The Terraform code deploys the following resources:
- 1 ClickHouse service on AWS
- 1 UDF (`clickhouse_udf`), a trivial Python executable that echoes its input, built from a local ZIP archive
- 1 UDF attachment (`clickhouse_udf_attachment`) wiring the UDF to the service

> **Note:** `clickhouse_udf` and `clickhouse_udf_attachment` are in beta and their behavior may change in future provider versions.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
