*WARNING:* This is an alpha resource. Specification can change at any time and no backward compatibilty is guaranteed at this stage.

You can use the `clickhouse_grant_role` resource to grant a `clickhouse_role` to either a `clickhouse_user` or to another `clickhouse_role`.

Known limitations:

- It's not possible to grant the same `clickhouse_role` to both a `clickhouse_user` and a `clickhouse_role` using a single `clickhouse_grant_role` stanza. You can do that using two different stanzas, one with `grantee_user_name` and the other with `grantee_role_name` fields set.
- Importing `clickhouse_grant_role` resources into terraform is not supported.
