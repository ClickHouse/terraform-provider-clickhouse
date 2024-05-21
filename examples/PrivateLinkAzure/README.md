## Azure Private Link example

Tested with hashicorp/azurerm v3.104.2 Terraform provider. 

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

### Important note

Azure's Terraform provider does not expose the Private Endpoint UUID, which means you need to run Terraform twice:
1. First Run: run terraform without setting `private_endpoint_azure_bar_uuid` & `private_endpoint_azure_foo_uuid` variables. This step creates services / DNS zones / Private Endpoints.
2. [Obtain Private Endpoint UUID](https://clickhouse.com/docs/en/cloud/security/azure-privatelink#obtaining-private-endpoint-resourceguid) for foo and bar endpoints.
3. Second Run: set `private_endpoint_azure_bar_uuid` `private_endpoint_azure_foo_uuid`. This time Private Endpoints will be added to organization and instance allow list.

There is [an open issue](https://github.com/hashicorp/terraform-provider-azurerm/issues/17011) to address this problem.