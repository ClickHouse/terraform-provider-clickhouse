## AWS Private Link example

Tested with HashiCorp/AWS v5.35.0 Terraform provider.

The Terraform code deploys following resources:
- 2 AWS PrivateLink endpoints with security groups: pl_vpc_foo & pl_vpc_bar
- 2 ClickHouse services: red & blue

The ClickHouse service "red" is available from `pl_vpc_foo` PrivateLink connection only, access from the internet is blocked. The ClickHouse service "blue" is available from `pl_vpc_foo`, `pl_vpc_bar` PrivateLink connections and also from the internet(0.0.0.0/0).