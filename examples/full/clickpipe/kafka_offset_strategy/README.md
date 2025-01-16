## ClickPipe Kafka with offset strategy example

This example demonstrates how to deploy a Kafka ClickPipe with a Kafka cluster as the input source,
with additional configuration for the Kafka offset strategy. 

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
