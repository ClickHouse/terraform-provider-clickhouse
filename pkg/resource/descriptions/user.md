You can use the `clickhouse_user` resource to create a user in a `ClickHouse cloud` service.

Known limitations:

- For security reasons, it is not possible to detect if the password for a user was changed outside terraform. Once first created, the password will not be checked for external changes.
- Any change to the `user` resource definition will cause the user to be deleted and recreated.
- Import of a `user` resource is not supported.
