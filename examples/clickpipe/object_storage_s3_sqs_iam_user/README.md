## ClickPipe Object Storage: Unordered Mode with IAM user example

This example demonstrates how to deploy a ClickPipe with S3 continuous ingestion using unordered mode (event-based via SQS) with IAM user authentication.

## How to run

- Rename `variables.sample.tfvars` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
