## Azure Private Link example

Tested with hashicorp/azurerm v3.107.0 Terraform provider. 

The Terraform example code deploys the following resources:
- VNET vnet-foo
- Private DNS zone `"${var.clickhouse_service_location}.privatelink.azure.clickhouse.cloud"` & link to vnet-foo
- Private Endpoint example-pl-foo in subnet vnet-foo/default
- DNS wildcard record pointed to Private Endpoint "example-pl-foo"
- VNET vent-bar
- Private DNS zone `"${var.clickhouse_service_location}.privatelink.azure.clickhouse.cloud"` & link to vnet-bar
- Private Endpoint example-pl-bar in subnet vnet-bar/default
- DNS wildcard record pointed to Private Endpoint "example-pl-bar"
- ClickHouse service red
- ClickHouse service blue



The ClickHouse service "red" is reachable via Private Link only from VNET bar, access from the internet is blocked.
The ClickHouse service "blue" is available any IP; access via Private Link is allowed from VNET foo and bar.
