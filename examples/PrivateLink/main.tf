terraform {
  required_providers {
    clickhouse = {
      version = "0.0.5"
      source  = "ClickHouse/clickhouse"
    }
  }
}

variable "organization_id" {
  type = string
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

resource "clickhouse_service" "aws_red" {
  name           = "red"
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

  // allow connections via PrivateLink from VPC foo only
  private_endpoint_ids = [clickhouse_private_endpoint_registration.private_endpoint_aws_foo.id, ]
}

resource "clickhouse_service" "aws_blue" {
  name           = "blue"
  cloud_provider = "aws"
  region         = var.aws_region
  tier           = "production"
  idle_scaling   = true
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0/0"
      description = "Any IP"
    }
  ]

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  // allow connecting via PrivateLink from VPC foo and bar
  private_endpoint_ids = [clickhouse_private_endpoint_registration.private_endpoint_aws_foo.id, clickhouse_private_endpoint_registration.private_endpoint_aws_bar.id]
}

// Private Link Service name for aws/${var.aws_region}
data "clickhouse_private_endpoint_config" "endpoint_config" {
  cloud_provider = "aws"
  region         = var.aws_region
}

// add AWS PrivateLink from VPC foo to organization
resource "clickhouse_private_endpoint_registration" "private_endpoint_aws_foo" {
  cloud_provider = "aws"
  id             = aws_vpc_endpoint.pl_vpc_foo.id
  region         = var.aws_region
  description    = "Private Link from VPC foo"
}

// add AWS PrivateLink from VPC bar to organization
resource "clickhouse_private_endpoint_registration" "private_endpoint_aws_bar" {
  cloud_provider = "aws"
  id             = aws_vpc_endpoint.pl_vpc_bar.id
  region         = var.aws_region
  description    = "Private Link from VPC bar"
}

// hostname for connecting to instance via PrivateLink from VPC foo
output "red_private_link_endpoint" {
  value = clickhouse_service.aws_red.private_endpoint_config.private_dns_hostname
}

// hostname for connecting to instance via PrivateLink from VPC foo & bar
output "blue_private_link_endpoint" {
  value = clickhouse_service.aws_blue.private_endpoint_config.private_dns_hostname
}
