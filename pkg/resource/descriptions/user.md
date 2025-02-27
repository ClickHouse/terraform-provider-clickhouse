*WARNING:* This is an alpha resource. Specification can change at any time and no backward compatibilty is guaranteed at this stage.

You can use the `clickhouse_user` resource to create a user in a `ClickHouse cloud` service.

Attention: in order to use the `clickhouse_user` resource you need to set the `query_api_endpoint` attribute in the `clickhouse_service`.
Please check [full example](https://github.com/ClickHouse/terraform-provider-clickhouse/blob/main/examples/rbac/main.tf).

Known limitations:

- For security reasons, it is not possible to detect if the password for a user was changed outside terraform. Once first created, the password will not be checked for external changes.
- Any change to the `user` resource definition will cause the user to be deleted and recreated.
- Import of a `user` resource is not supported.
