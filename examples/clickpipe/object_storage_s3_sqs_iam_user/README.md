## ClickPipe Object Storage with S3/SQS/IAM user example

This example demonstrates how to deploy a ClickPipe with an S3 bucket as the input source with continuous ingestion using SQS event notifications, authenticated with an IAM user.

This setup enables event-based ingestion where new files are detected via S3 event notifications sent to an SQS queue, rather than polling S3 for new files.

## How to run

- Rename `variables.sample.tfvars` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
