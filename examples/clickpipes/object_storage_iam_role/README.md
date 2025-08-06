## ClickPipe Object Storage with S3/IAM role example

This example demonstrates how to deploy a ClickPipe with an S3 bucket as the input source,
authenticated with an IAM role.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
