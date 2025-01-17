You can use the *clickhouse_service* resource to deploy ClickHouse cloud instances on supported cloud providers.

Known limitations:

- If you create a service with `warehouse_id` set and then remove `warehouse_id` attribute completely, the provider won't detect the change. If you want to make a secondary service become primary, remove the `warehouse_id` and taint it before applying.
- If you create a service with `readonly` flag set to true and then remove `readonly` flag completely, the provider won't detect the change. If you want to make a secondary service read write, explicitly set the `readonly` flag to false.
