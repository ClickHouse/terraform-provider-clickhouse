# AWS Private Link example

Tested with HashiCorp/AWS v5.63.0 Terraform provider.

The ClickHouse service is available from the subnet, access from the internet is blocked.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
