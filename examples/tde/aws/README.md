## AWS Basic example

The Terraform code deploys following resources:
- 1 ClickHouse service on AWS

WARNING: this example requires an organization with the Enterprise plan to run.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
