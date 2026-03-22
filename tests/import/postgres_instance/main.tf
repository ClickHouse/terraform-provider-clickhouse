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
  type = string
}

variable "cloud_provider" {
  type    = string
  default = "aws"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

resource "clickhouse_postgres_instance" "import" {
  name           = var.service_name
  cloud_provider = var.cloud_provider
  region         = var.region
  size           = "m6gd.medium"
  storage_size   = 118
  ha_type        = "none"

  pg_config = {
    wal_level              = "logical"
    hot_standby_feedback   = "on"
    sync_replication_slots = "on"
  }
}

output "instance_id" {
  value = clickhouse_postgres_instance.import.id
}
