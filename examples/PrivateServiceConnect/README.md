## GCP Private Service Connect example

Tested with HashiCorp/Google v5.15.0 Terraform module. 

The Terraform example code deploys the following resources:
- GCP Private Service Connect endpoint & endpoint IP address
- Private DNS zone & wildcard DNS record
- 2 ClickHouse services: red & blue

The ClickHouse service "blue" is reachable via Private Service Connect link only, access from the internet is blocked. The ClickHouse service "red" is available from 1.1.1.1 IP; access via Private Service Connect is not allowed.