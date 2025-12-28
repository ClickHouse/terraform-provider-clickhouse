## ClickPipe BigQuery snapshot example

This example demonstrates how to deploy a BigQuery snapshot ClickPipe using Terraform.

It provisions all necessary GCP prerequisites, including:
- GCS staging bucket
- IAM service account with required permissions
- IAM service account key

BigQuery dataset and table must already exist.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
