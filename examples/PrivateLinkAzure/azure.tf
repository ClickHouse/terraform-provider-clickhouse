provider "azurerm" {
  features {}
}


variable "resource_group_foo" {
  type = string
}

variable "resource_group_bar" {
  type = string
}

resource "azurerm_private_dns_zone" "clickhouse_cloud_private_link_zone_foo" {
  name                = "${var.clickhouse_service_location}.privatelink.azure.clickhouse.cloud"
  resource_group_name = var.resource_group_foo
}

resource "azurerm_private_dns_a_record" "wildcard_foo" {
  name                = "*"
  zone_name           = azurerm_private_dns_zone.clickhouse_cloud_private_link_zone_foo.name
  resource_group_name = var.resource_group_foo
  ttl                 = 300
  records             = [data.azurerm_network_interface.pe_foo.private_ip_address]
}


resource "azurerm_private_dns_zone" "clickhouse_cloud_private_link_zone_bar" {
  name = "${var.clickhouse_service_location}.privatelink.azure.clickhouse.cloud"
  // azure does not allow creating 2 the same private DNS zones within the same resource group
  resource_group_name = var.resource_group_bar
}

resource "azurerm_private_dns_a_record" "wildcard_bar" {
  name                = "*"
  zone_name           = azurerm_private_dns_zone.clickhouse_cloud_private_link_zone_foo.name
  resource_group_name = var.resource_group_bar
  ttl                 = 300
  records             = [data.azurerm_network_interface.pe_foo.private_ip_address]
}


resource "azurerm_virtual_network" "vnet_foo" {
  name                = "vnet-foo"
  address_space       = ["10.0.0.0/16"]
  location            = "eastus"
  resource_group_name = var.resource_group_foo

  subnet {
    name           = "default"
    address_prefix = "10.0.0.0/24"
  }
}

resource "azurerm_virtual_network" "vnet_bar" {
  name                = "vnet-bar"
  address_space       = ["10.0.0.0/16"]
  location            = "eastus2"
  resource_group_name = var.resource_group_bar

  subnet {
    name           = "default"
    address_prefix = "10.0.0.0/24"
  }
}

resource "azurerm_private_dns_zone_virtual_network_link" "vnet_foo" {
  name                  = "test"
  resource_group_name   = var.resource_group_foo
  private_dns_zone_name = azurerm_private_dns_zone.clickhouse_cloud_private_link_zone_foo.name
  virtual_network_id    = azurerm_virtual_network.vnet_foo.id
}

resource "azurerm_private_dns_zone_virtual_network_link" "vnet_bar" {
  name                  = "test"
  resource_group_name   = var.resource_group_bar
  private_dns_zone_name = azurerm_private_dns_zone.clickhouse_cloud_private_link_zone_bar.name
  virtual_network_id    = azurerm_virtual_network.vnet_bar.id
}

resource "azurerm_private_endpoint" "foo_example_clickhouse_cloud" {
  name = "example-pl-foo"
  // make sure location of azurerm_private_endpoint matches of location of vnet_foo_private_link_subnet_id
  location            = "eastus"
  resource_group_name = var.resource_group_foo
  subnet_id           = "${azurerm_virtual_network.vnet_foo.id}/subnets/default"
  private_service_connection {
    name                              = "example-pl-foo"
    request_message                   = "please approve"
    private_connection_resource_alias = data.clickhouse_private_endpoint_config.endpoint_config.endpoint_service_id
    is_manual_connection              = true
  }
}

resource "azurerm_private_endpoint" "bar_example_clickhouse_cloud" {
  name = "example-pl-bar"
  // make sure location of azurerm_private_endpoint matches of location of vnet_foo_private_link_subnet_id
  location            = "eastus2"
  resource_group_name = var.resource_group_bar
  subnet_id           = "${azurerm_virtual_network.vnet_bar.id}/subnets/default"
  private_service_connection {
    name                              = "example-pl-bar"
    request_message                   = "please approve"
    private_connection_resource_alias = data.clickhouse_private_endpoint_config.endpoint_config.endpoint_service_id
    is_manual_connection              = true
  }
}


data "azurerm_network_interface" "pe_foo" {
  resource_group_name = var.resource_group_foo
  name                = azurerm_private_endpoint.foo_example_clickhouse_cloud.network_interface[0].name
}

data "azurerm_network_interface" "pe_bar" {
  resource_group_name = var.resource_group_bar
  name                = azurerm_private_endpoint.bar_example_clickhouse_cloud.network_interface[0].name
}