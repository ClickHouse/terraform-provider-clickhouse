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
}

variable "cloud_provider" {
  type = string
}

variable "region" {
  type = string
}

variable "release_channel" {
  type = string
  default = "default"
  validation {
    condition     = var.release_channel == "default" || var.release_channel == "fast"
    error_message = "Release channel can be either 'default' or 'fast'."
  }
}

resource "clickhouse_service" "import" {
  name                      = var.service_name
  cloud_provider            = var.cloud_provider
  region                    = var.region
  release_channel           = var.release_channel
  idle_scaling              = true
  idle_timeout_minutes      = 15
  password_hash             = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0/0"
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
}

output "service_uuid" {
  value = clickhouse_service.import.id
}
