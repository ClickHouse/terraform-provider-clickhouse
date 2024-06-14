terraform {
  required_providers {
    clickhouse = {
      version = "0.0.10"
      source  = "ClickHouse/clickhouse"
    }

    azapi = {
      source  = "Azure/azapi"
      version = "1.13.1"
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

variable "clickhouse_service_location" {
  description = "azure location where ClickHouse cloud instance is created"
  type        = string
}

variable "private_endpoint_azure_foo_uuid" {
  type    = string
  default = ""
}

variable "private_endpoint_azure_bar_uuid" {
  type    = string
  default = ""
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

resource "clickhouse_service" "azure_red" {
  name           = "red"
  cloud_provider = "azure"
  region         = var.clickhouse_service_location
  tier           = "production"
  idle_scaling   = true
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  // keep it empty to block access from internet
  ip_access = []

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  // allow connections via PrivateLink from VPC bar only
  private_endpoint_ids = [clickhouse_private_endpoint_registration.private_endpoint_azure_bar.id]
}

resource "clickhouse_service" "azure_blue" {
  name           = "blue"
  cloud_provider = "azure"
  region         = var.clickhouse_service_location
  tier           = "production"
  idle_scaling   = true
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0/0"
      description = "Any IP"
    }
  ]

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5

  // allow connecting via PrivateLink from VPC foo and bar
  private_endpoint_ids = [clickhouse_private_endpoint_registration.private_endpoint_azure_foo.id, clickhouse_private_endpoint_registration.private_endpoint_azure_bar.id]
}

// Private Link Service name for azure/${var.clickhouse_service_location}
data "clickhouse_private_endpoint_config" "endpoint_config" {
  cloud_provider = "azure"
  region         = var.clickhouse_service_location
}

resource "clickhouse_private_endpoint_registration" "private_endpoint_azure_foo" {
  cloud_provider = "azure"
  id             = jsondecode(data.azapi_resource.clickhouse_cloud_privateendpoint_resource_guid_foo.output).properties.resourceGuid
  region         = var.clickhouse_service_location
  description    = "Private Link from VNET foo"
}

resource "clickhouse_private_endpoint_registration" "private_endpoint_azure_bar" {
  cloud_provider = "azure"
  id             = jsondecode(data.azapi_resource.clickhouse_cloud_privateendpoint_resource_guid_bar.output).properties.resourceGuid
  region         = var.clickhouse_service_location
  description    = "Private Link from VNET foo"
}

// hostname for connecting to instance via Private Link from VPC foo
output "red_private_link_endpoint" {
  value = clickhouse_service.azure_red.private_endpoint_config.private_dns_hostname
}

// hostname for connecting to instance via Private Link from VPC foo & bar
output "blue_private_link_endpoint" {
  value = clickhouse_service.azure_blue.private_endpoint_config.private_dns_hostname
}
