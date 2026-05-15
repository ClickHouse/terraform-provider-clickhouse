You can use the *clickhouse_service_scheduled_scaling* resource to manage time-based scaling rules for a ClickHouse Cloud service.

A schedule is a set of recurring weekly windows. The server rejects any pair of entries that overlap in time, so at most one window is active at any moment. While a window is active the service uses the replica count, memory bounds, and idle-scaling settings declared on that entry; otherwise the service falls back to its base auto-scaling configuration.

~> **Note:** This resource is in beta. Scheduled scaling must be enabled for your organization (`canUseScheduledAutoscaling`). Reach out to ClickHouse support if the API returns `403 FORBIDDEN`. The server currently requires `min_replicas == max_replicas` per entry, and a maximum of 10 entries per schedule.

## Hour ranges

Hour ranges are asymmetric:

- `start_hour_utc` accepts `0`–`23`.
- `end_hour_utc` accepts `1`–`24`.
- `start_hour_utc` and `end_hour_utc` must differ.
- Set `start_hour_utc = 0` and `end_hour_utc = 24` for a 24-hour window.
- Set `end_hour_utc < start_hour_utc` to wrap overnight (e.g. `22` to `6` covers 22:00–06:00 next day).

## Example Usage

```hcl
resource "clickhouse_service_scheduled_scaling" "example" {
  service_id = clickhouse_service.example.id

  entries = [
    {
      name           = "Business hours"
      weekdays       = [1, 2, 3, 4, 5] # Mon-Fri
      start_hour_utc = 8
      end_hour_utc   = 18
      min_replicas   = 3
      max_replicas   = 3
      idle_scaling   = false
    },
    {
      name                 = "Overnight"
      weekdays             = [0, 1, 2, 3, 4, 5, 6]
      start_hour_utc       = 22
      end_hour_utc         = 6 # wraps overnight when end < start
      min_replicas         = 1
      max_replicas         = 1
      idle_scaling         = true
      idle_timeout_minutes = 5
    },
  ]
}
```

## Import

```sh
terraform import clickhouse_service_scheduled_scaling.example <service_id>
```
