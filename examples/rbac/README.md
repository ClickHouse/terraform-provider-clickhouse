## Basic example to manage Role Based Access Control

The Terraform code deploys following resources:
- 1 ClickHouse service on AWS
- 1 database user on the ClickHouse service
- 1 database role on the ClickHouse service

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
