You can use the *clickhouse_service* resource to deploy ClickHouse cloud instances on supported cloud providers.

Known limitations:

- If you create a service with `warehouse_id` set and then remove `warehouse_id` attribute completely, the provider won't detect the change. If you want to make a secondary service become primary, remove the `warehouse_id` and taint it before applying.
- If you create a service with `readonly` flag set to true and then remove `readonly` flag completely, the provider won't detect the change. If you want to make a secondary service read write, explicitly set the `readonly` flag to false.

Stopping a service:

- Setting `stop = true` stops the service: its compute is fully shut down so compute billing stops (storage is still billed). Unlike auto-idle (see `idle_scaling`), a stopped service does NOT auto-resume on query; set `stop = false` to start it again.
- Secondary (replica) services in a warehouse can be stopped independently — stopping one only shuts down that service's compute; the primary and sibling services keep running.
- The primary (first) service of a warehouse cannot be stopped while any secondary services still exist: ClickHouse Cloud requires the first service to stay up. Remove the secondary services before stopping the primary.
