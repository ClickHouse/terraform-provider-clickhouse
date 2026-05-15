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
  default = "us-east-2"
}

variable "release_channel" {
  type    = string
  default = "default"
  validation {
    condition     = contains(["default", "fast", "slow"], var.release_channel)
    error_message = "Release channel can be 'default', 'fast' or 'slow'."
  }
}

resource "clickhouse_service" "service" {
  name                 = var.service_name
  cloud_provider       = "aws"
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

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120

  backup_configuration = {
    backup_period_in_hours           = 24
    backup_retention_period_in_hours = 24
    backup_start_time                = null
  }
}

resource "clickhouse_service_scheduled_scaling" "schedule" {
  service_id = clickhouse_service.service.id

  entries = [
    {
      name           = "Business hours"
      weekdays       = [1, 2, 3, 4, 5]
      start_hour_utc = 8
      end_hour_utc   = 18
      min_replicas   = 3
      max_replicas   = 3
      idle_scaling   = false
    },
    {
      name                 = "Overnight"
      weekdays             = [0, 1, 2, 3, 4, 5, 6]
      start_hour_utc       = 22
      end_hour_utc         = 6
      min_replicas         = 1
      max_replicas         = 1
      idle_scaling         = true
      idle_timeout_minutes = 5
    },
  ]
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "service_iam" {
  value = clickhouse_service.service.iam_role
}
