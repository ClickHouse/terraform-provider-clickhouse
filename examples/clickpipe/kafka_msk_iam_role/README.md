## ClickPipe Kafka with MSK/IAM role

This example demonstrates how to deploy a Kafka ClickPipe with a MSK cluster as the input source,
authenticated with an IAM role.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
