resource "clickhouse_private_endpoint_registration" "endpoint" {
  cloud_provider      = "aws"
  private_endpoint_id = "vpce-..."
  region              = "us-west-2"
  description         = "Private Link from VPC foo"
}
