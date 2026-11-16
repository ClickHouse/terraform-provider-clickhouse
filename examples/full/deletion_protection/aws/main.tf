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

# The e2e update pass sets this to true, which turns protection off so the service can be destroyed.
variable "update_pass" {
  type    = bool
  default = false
}

resource "clickhouse_service" "service" {
  name                 = var.service_name
  cloud_provider       = "aws"
  region               = var.region
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

  # Set to false and apply before destroying the service.
  deletion_protection = !var.update_pass
}

output "service_id" {
  value = clickhouse_service.service.id
}

output "deletion_protection" {
  value = clickhouse_service.service.deletion_protection
}
