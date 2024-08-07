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
  default = "My Terraform Service"
}

variable "cloud_provider" {
  type = string
  default = "aws"
}

locals {
  regions = {
    aws = "us-east-1"
    gcp = "europe-west4"
    azure = "westus3"
  }
}

resource "clickhouse_service" "service" {
  name                      = var.service_name
  cloud_provider            = var.cloud_provider
  region                    = local.regions[var.cloud_provider]
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
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "service_iam" {
  value = clickhouse_service.service.iam_role
}
