*WARNING:* This is an alpha resource. Specification can change at any time and no backward compatibilty is guaranteed at this stage.

You can use the `clickhouse_role` resource to create a `role` in a `ClickHouse cloud` service.

Attention: in order to use the `clickhouse_role` resource you need to set the `query_api_endpoint` attribute in the `clickhouse_service`.
Please check [full example](https://github.com/ClickHouse/terraform-provider-clickhouse/blob/main/examples/rbac/main.tf).
