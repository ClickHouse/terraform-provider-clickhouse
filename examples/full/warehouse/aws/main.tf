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
  default = "My Data Warehouse"
}

variable "region" {
  type = string
  default = "us-east-2"
}

resource "clickhouse_service" "primary" {
  name                      = "${var.service_name}-primary"
  cloud_provider            = "aws"
  region                    = var.region
  num_replicas              = 3
  idle_scaling              = false
  idle_timeout_minutes      = null
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
}

resource "clickhouse_service" "secondary" {
  warehouse_id              = clickhouse_service.primary.warehouse_id
  readonly                  = true
  name                      = "${var.service_name}-secondary"
  cloud_provider            = "aws"
  region                    = var.region
  num_replicas              = 1
  idle_scaling              = true
  idle_timeout_minutes      = 5

  ip_access = [
    {
      source      = "0.0.0.0"
      description = "Anywhere"
    }
  ]

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120
}
