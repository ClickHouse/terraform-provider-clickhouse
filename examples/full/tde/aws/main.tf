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

variable "region" {
  type = string
  default = "us-east-2"
}

variable "release_channel" {
  type = string
  default = "default"
  validation {
    condition     = var.release_channel == "default" || var.release_channel == "fast"
    error_message = "Release channel can be either 'default' or 'fast'."
  }
}

data "clickhouse_api_key_id" "self" {
}

resource "clickhouse_service" "service" {
  name                      = var.service_name
  cloud_provider            = "aws"
  region                    = var.region
  release_channel           = var.release_channel
  idle_scaling              = true
  idle_timeout_minutes      = 5
  password_hash             = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0"
      description = "Anywhere"
    }
  ]

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120

  backup_configuration = {
    backup_period_in_hours           = 24
    backup_retention_period_in_hours = 24
    backup_start_time                = null
  }

  transparent_data_encryption = {
    enabled = true
  }
}

resource "clickhouse_service_transparent_data_encryption_key_association" "service_key_association" {
  service_id = clickhouse_service.service.id
  key_id = aws_kms_key.enc.arn
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "service_iam" {
  value = clickhouse_service.service.iam_role
}
