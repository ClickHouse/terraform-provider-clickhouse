## AWS Private Link example

Tested with HashiCorp/AWS v5.35.0 Terraform provider.

The Terraform code deploys following resources:
- 1 AWS PrivateLink endpoint with security groups: pl_vpc_foo
- 1 ClickHouse service: red

The ClickHouse service "red" is available from `pl_vpc_foo` PrivateLink connection only, access from the internet is blocked.
