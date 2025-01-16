## ClickPipe Kafka example

The Terraform code deploys following resources:
- 1 ClickPipe Kafka on ClickHouse Cloud

The ClickHouse service is available from anywhere.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
