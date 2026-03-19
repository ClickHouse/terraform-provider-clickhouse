## ClickPipe Object Storage: GCS Unordered Mode with Service Account example

This example demonstrates how to deploy a ClickPipe with GCS continuous ingestion using unordered mode (event-based via Pub/Sub) with service account authentication.

## How to run

- Rename `variables.sample.tfvars` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
