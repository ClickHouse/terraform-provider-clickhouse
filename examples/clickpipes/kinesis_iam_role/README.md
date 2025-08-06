## ClickPipe Kinesis with IAM role authentication

This example demonstrates how to deploy a Kinesis ClickPipe using IAM role authentication. This method allows ClickHouse to securely access your Kinesis streams without needing to store access keys.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
