terraform {
  required_providers {
    clickhouse = {
      version = "0.0.5"
      source  = "ClickHouse/clickhouse"
    }
  }
}

variable "organization_id" {
  type = string
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

resource "clickhouse_service" "gcp_red" {
  name           = "gcp_red"
  cloud_provider = "gcp"
  region         = var.gcp_region
  tier           = "production"
  idle_scaling   = true
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "1.1.1.1/32"
      description = "Test IP"
    }
  ]

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  // Private Service Connect filter is empty
  private_endpoint_ids = []
}

resource "clickhouse_service" "gcp_blue" {
  name           = "gcp_blue"
  cloud_provider = "gcp"
  region         = var.gcp_region
  tier           = "production"
  idle_scaling   = true
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  // block access to the service from internet
  ip_access = [
  ]

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  // allow access via Private Service Connect
  private_endpoint_ids = [clickhouse_private_endpoint_registration.private_endpoint_gcp.id]
}

// add GCP PSC to organization config
resource "clickhouse_private_endpoint_registration" "private_endpoint_gcp" {
  cloud_provider = "gcp"
  id             = google_compute_forwarding_rule.clickhouse_cloud_psc.psc_connection_id
  region         = var.gcp_region
  description    = "PSC connection from project ${var.gcp_project_id}"
}

// endpoint for via Private Service Connect
output "blue_pl_endpoint" {
  value = clickhouse_service.gcp_blue.private_endpoint_config.private_dns_hostname
}