variable "organization_id" {
  type = string
}

variable "token_key" {
  type      = string
  sensitive = true
}

variable "token_secret" {
  type      = string
  sensitive = true
}

variable "service_name" {
  type    = string
  default = "My Terraform Service"
}

variable "region" {
  type    = string
  default = "westus3"
}

variable "release_channel" {
  type    = string
  default = "fast"
  validation {
    condition     = contains(["default", "fast", "slow"], var.release_channel)
    error_message = "Release channel can be 'default', 'fast' or 'slow'."
  }
}

data "clickhouse_api_key_id" "self" {
}

resource "clickhouse_service" "service" {
  name                 = var.service_name
  cloud_provider       = "azure"
  region               = var.region
  release_channel      = var.release_channel
  idle_scaling         = true
  idle_timeout_minutes = 5
  password_hash        = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

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
    backup_retention_period_in_hours = 24
    backup_start_time                = "02:00"
  }
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "service_iam" {
  value = clickhouse_service.service.iam_role
}
