## AWS Compute-Compute separation example

The Terraform code deploys following resources:
- 1 ClickHouse service on AWS meant to be the primary service of a data warehouse
- 1 ClickHouse service on AWS meant to be the secondary service in the same data warehouse

The ClickHouse services will be reachable from anywhere on the internet.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
