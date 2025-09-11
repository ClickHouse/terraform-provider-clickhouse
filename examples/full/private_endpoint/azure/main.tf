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

variable "location" {
  type    = string
  default = "westus3"
}

resource "clickhouse_service" "this" {
  name                 = var.service_name
  cloud_provider       = "azure"
  region               = var.location
  idle_scaling         = true
  idle_timeout_minutes = 5
  password_hash        = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  // keep it empty to block access from internet
  ip_access = []

  backup_configuration = {
    backup_period_in_hours           = 24
    backup_retention_period_in_hours = 24
    backup_start_time                = null
  }

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120
}

resource "clickhouse_service_private_endpoints_attachment" "this" {
  private_endpoint_ids = [azurerm_private_endpoint.this.id]
  service_id           = clickhouse_service.this.id
}

# hostname for connecting to instance via PrivateLink from Vnet
output "private_link_endpoint" {
  value = clickhouse_service.this.private_endpoint_config.private_dns_hostname
}
