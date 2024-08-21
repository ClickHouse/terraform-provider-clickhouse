variable "organization_id" {
  type = string
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

variable "service_name" {
  type = string
  default = "red"
}

resource "clickhouse_service" "aws_red" {
  name           = var.service_name
  cloud_provider = "aws"
  region         = var.aws_region
  tier           = "production"
  idle_scaling   = true
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  // keep it empty to block access from internet
  ip_access = []

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5
}

// add AWS PrivateLink from VPC foo to organization
resource "clickhouse_private_endpoint_registration" "private_endpoint_aws_foo" {
  cloud_provider = "aws"
  id             = aws_vpc_endpoint.pl_vpc_foo.id
  region         = var.aws_region
  description    = "Private Link from VPC foo"
}

resource "clickhouse_service_private_endpoints_attachment" "red_attachment" {
  private_endpoint_ids = [clickhouse_private_endpoint_registration.private_endpoint_aws_foo.id]
  service_id = clickhouse_service.aws_red.id
}

data "clickhouse_private_endpoint_config" "endpoint_config" {
  cloud_provider = "aws"
  region         = var.aws_region

  depends_on = [clickhouse_service.aws_red]
}

// hostname for connecting to instance via PrivateLink from VPC foo
output "red_private_link_endpoint" {
  value = clickhouse_service.aws_red.private_endpoint_config.private_dns_hostname
}
