## AWS Private Link example

Tested with HashiCorp/AWS v5.63.0 Terraform provider.

The Terraform code deploys following resources:
- 1 AWS PrivateLink endpoint with security groups: pl_vpc_foo
- 1 ClickHouse service: red

The ClickHouse service is available from `pl_vpc_foo` PrivateLink connection only, access from the internet is blocked.

## How to run

- Create a VPC into AWS
- Create 2 subnets within the VPC in 2 different AZs.
- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
