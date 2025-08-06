## ClickPipe Object Storage with S3/IAM user example

This example demonstrates how to deploy a ClickPipe with an S3 bucket as the input source,
authenticated with an IAM user.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
