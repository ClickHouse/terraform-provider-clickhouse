variable "subscription_id" {
  type      = string
  sensitive = true
}

provider "azurerm" {
  subscription_id = var.subscription_id
  features {
    resource_group {
      # Dangerous!! Remove this to prevent force delete a resource group
      prevent_deletion_if_contains_resources = false
    }
  }
}

locals {
  tags = {
    Name = var.service_name
  }
  service_name_normalized = replace(var.service_name, "/\\[|\\]/", "")
}

resource "azurerm_resource_group" "this" {
  name     = local.service_name_normalized
  location = var.region
  tags     = local.tags
}

resource "azurerm_virtual_network" "this" {
  name                = local.service_name_normalized
  address_space       = ["10.0.0.0/16"]
  location            = var.region
  resource_group_name = azurerm_resource_group.this.name
  tags                = local.tags
}

resource "azurerm_subnet" "this" {
  name                 = local.service_name_normalized
  resource_group_name  = azurerm_resource_group.this.name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_private_endpoint" "this" {
  name                = local.service_name_normalized
  location            = var.region
  resource_group_name = azurerm_resource_group.this.name
  subnet_id           = azurerm_subnet.this.id

  private_service_connection {
    name                              = local.service_name_normalized
    private_connection_resource_alias = clickhouse_service.this.private_endpoint_config.endpoint_service_id
    is_manual_connection              = true
    request_message                   = "clickhouse-${local.service_name_normalized}"
  }

  tags = local.tags
}

resource "azurerm_network_security_group" "this" {
  name                = local.service_name_normalized
  location            = var.region
  resource_group_name = azurerm_resource_group.this.name
  tags                = local.tags
}

resource "azurerm_subnet_network_security_group_association" "this" {
  subnet_id                 = azurerm_subnet.this.id
  network_security_group_id = azurerm_network_security_group.this.id
}

resource "azurerm_network_security_rule" "this" {
  name              = local.service_name_normalized
  description       = "Allow subnet to ${local.service_name_normalized}"
  priority          = 100
  direction         = "Inbound"
  access            = "Allow"
  protocol          = "Tcp"
  source_port_range = "*"
  destination_port_ranges = [
    "8443", # https
    "9440", # native
  ]
  source_address_prefixes      = azurerm_virtual_network.this.address_space
  destination_address_prefixes = azurerm_private_endpoint.this.private_service_connection[*].private_ip_address
  resource_group_name          = azurerm_resource_group.this.name
  network_security_group_name  = azurerm_network_security_group.this.name
}

resource "azurerm_private_dns_zone" "clickhouse_cloud_private_link_zone" {
  # Extract fqdn domain from private hostname
  name                = regex("^[^.]+\\.(.+)$", clickhouse_service.this.private_endpoint_config.private_dns_hostname)[0]
  resource_group_name = azurerm_resource_group.this.name
}

resource "azurerm_private_dns_a_record" "this" {
  name                = "*"
  zone_name           = azurerm_private_dns_zone.clickhouse_cloud_private_link_zone.name
  resource_group_name = azurerm_resource_group.this.name
  ttl                 = 300
  records             = azurerm_private_endpoint.this.private_service_connection[*].private_ip_address
}

resource "azurerm_private_dns_zone_virtual_network_link" "this" {
  name                  = "clickhouse-private-link"
  resource_group_name   = azurerm_resource_group.this.name
  private_dns_zone_name = azurerm_private_dns_zone.clickhouse_cloud_private_link_zone.name
  virtual_network_id    = azurerm_virtual_network.this.id
}
