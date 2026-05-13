You can use the *clickhouse_service_scheduled_scaling* resource to manage time-based scaling rules for a ClickHouse Cloud service.

A schedule is a list of recurring weekly windows. While a window is active the service uses the replica count, memory bounds, and idle-scaling settings declared on that entry; otherwise the service falls back to its base auto-scaling configuration.

~> **Note:** This resource is in beta. Scheduled scaling must be enabled for your organization (`canUseScheduledAutoscaling`). Reach out to ClickHouse support if the API returns `403 FORBIDDEN`. The server currently requires `min_replicas == max_replicas` per entry.

## Example Usage

```hcl
resource "clickhouse_service_scheduled_scaling" "example" {
  service_id = clickhouse_service.example.id

  entries = [
    {
      name           = "Business hours"
      weekdays       = [1, 2, 3, 4, 5] # Mon–Fri
      start_hour_utc = 8
      end_hour_utc   = 18
      min_replicas   = 3
      max_replicas   = 3
      idle_scaling   = false
    },
    {
      name           = "Overnight"
      weekdays       = [0, 1, 2, 3, 4, 5, 6]
      start_hour_utc = 22
      end_hour_utc   = 6 # wraps overnight when end < start
      min_replicas   = 1
      max_replicas   = 1
      idle_scaling   = true
      idle_timeout_minutes = 5
    },
  ]
}
```

## Import

```sh
terraform import clickhouse_service_scheduled_scaling.example <service_id>
```
