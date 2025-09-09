# AWS Private Link example

Tested with HashiCorp/AWS v5.63.0 Terraform provider.

The Terraform code deploys following resources:

- 1 Azure PrivateLink endpoint with security groups: pl_vpc_foo
- 1 ClickHouse service: red

The ClickHouse service is available from the subnet, access from the internet is blocked.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`

## Needed Azure permissions

To run this example, the Azure user you provide credentials for needs the following permissions:

TODO
