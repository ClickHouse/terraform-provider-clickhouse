## AWS Private Link example
The terraform code deploys following resources:
- 2 AWS PrivateLink endpoints with security groups: pl_vpc_foo & pl_vpc_bar
- 2 ClickHouse services: red & blue

The ClickHouse service "red" is available from `pl_vpc_foo` PrivateLink connection and 8.8.8.8 IP. The ClickHouse service "blue" is available from `pl_vpc_foo`, `pl_vpc_bar` PrivateLink connections and also from the internet(0.0.0.0/0).