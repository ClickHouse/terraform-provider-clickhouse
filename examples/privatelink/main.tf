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

provider clickhouse {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

# register PrivateLink endpoint ids here
resource "clickhouse_private_endpoint_registration" "private_endpoints" {
  private_endpoints = [
    {
      cloud_provider = "aws"
      id             = "vpce-abcdef12345678910"
      region         = "us-east-1"
    }
  ]
}

resource "clickhouse_service" "service" {
  name                      = "My PrivateLink Service"
  cloud_provider            = "aws"
  region                    = "us-east-1"
  tier                      = "production"
  idle_scaling              = true
  password_hash             = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0"
      description = "Test IP"
    }
  ]

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  # use registered endpoint ids
  private_endpoint_ids = clickhouse_private_endpoint_registration.private_endpoints.private_endpoint_ids
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "private_endpoint_config" {
  value = clickhouse_service.service.private_endpoint_config
}
