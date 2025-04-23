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
  default = "europe-west4"
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
  cloud_provider            = "gcp"
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

  endpoints = {
    mysql = {
      enabled = true
    }
  }

  query_api_endpoints = {
    api_key_ids = [
      data.clickhouse_api_key_id.self.id,
    ]
    roles = [
      "sql_console_admin"
    ]
    allowed_origins = null
  }

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120

  backup_configuration = {
    backup_retention_period_in_hours = 48
  }

  transparent_data_encryption = {
    enabled = true
  }
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "service_iam" {
  value = clickhouse_service.service.iam_role
}
