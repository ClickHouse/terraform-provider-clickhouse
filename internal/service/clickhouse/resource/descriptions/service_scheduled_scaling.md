You can use the *clickhouse_service_scheduled_scaling* resource to manage time-based scaling rules for a ClickHouse Cloud service.

A schedule is a set of recurring weekly windows. The server rejects any pair of entries that overlap in time, so at most one window is active at any moment. While a window is active the service uses the replica count, memory bounds, and idle-scaling settings declared on that entry; otherwise the service falls back to its base auto-scaling configuration.

~> **Note:** This resource is in alpha. Scheduled scaling must be enabled for your organization (`canUseScheduledAutoscaling`). Reach out to ClickHouse support if the API returns `403 FORBIDDEN`. Each entry is vertical by default — a fixed replica count (`min_replicas == max_replicas`); set `autoscaling_mode = "horizontal"` to scale the replica count across a `min_replicas`–`max_replicas` band at fixed per-replica memory. A schedule allows a maximum of 10 entries.

## Hour ranges

Hour ranges are asymmetric:

- `start_hour_utc` accepts `0`–`23`.
- `end_hour_utc` accepts `1`–`24`.
- `start_hour_utc` and `end_hour_utc` must differ.
- Set `start_hour_utc = 0` and `end_hour_utc = 24` for a 24-hour window.
- Set `end_hour_utc < start_hour_utc` to wrap overnight (e.g. `22` to `6` covers 22:00–06:00 next day).
