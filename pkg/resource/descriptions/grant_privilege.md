*WARNING:* This is an alpha resource. Specification can change at any time and no backward compatibilty is guaranteed at this stage.

You can use the `clickhouse_grant_privilege` resource to grant privileges on databases and tables to either a `clickhouse_user` or a `clickhouse_role`.

Please note that in order to grant privileges to all database and/or all tables, the `database` and/or `table` fields must be set to null, and not to "*".

Attention: in order to use the `clickhouse_grant_privilege` resource you need to set the `query_api_endpoint` attribute in the `clickhouse_service`.
Please check [full example](https://github.com/ClickHouse/terraform-provider-clickhouse/blob/main/examples/rbac/main.tf).

Known limitations:

- Only a subset of privileges can be granted on ClickHouse cloud. For example the `ALL` privilege can't be granted. See https://clickhouse.com/docs/en/sql-reference/statements/grant#all
- It's not possible to grant the same `clickhouse_grant_privilege` to both a `clickhouse_user` and a `clickhouse_role` using a single `clickhouse_grant_privilege` stanza. You can do that using two different stanzas, one with `grantee_user_name` and the other with `grantee_role_name` fields set.
- It's not possible to grant the same privilege (example 'SELECT') to multiple entities (for example tables) with a single stanza. You can do that my creating one stanza for each entity you want to grant privileges on.
- Importing `clickhouse_grant_privilege` resources into terraform is not supported.
