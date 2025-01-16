## ClickPipe Kafka with MSK/IAM user

This example demonstrates how to deploy a Kafka ClickPipe with a MSK cluster as the input source,
authenticated with an IAM user.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
