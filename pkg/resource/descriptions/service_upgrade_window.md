You can use the *clickhouse_service_upgrade_window* resource to pin the weekly upgrade window for a ClickHouse Cloud service.

When configured, the data plane only attempts service upgrades during the declared window. The window is a single weekly recurrence: a `weekday` plus a `start_hour_utc`. The window currently lasts 6 hours from `start_hour_utc`; `duration` is returned by the API and exposed as a read-only attribute.

~> **Note:** This resource is in beta. Scheduled upgrades must be enabled for your organization (`canUseScheduledUpgrades`). Reach out to ClickHouse support if the API returns `403 FORBIDDEN`. The upgrade window can only be set on primary services; secondary services inherit the primary service window.

## Allowed values

- `weekday`: `0` (Sunday) – `6` (Saturday)
- `start_hour_utc`: one of `0`, `6`, `12`, `18`

## Example Usage

```hcl
resource "clickhouse_service_upgrade_window" "example" {
  service_id     = clickhouse_service.example.id
  weekday        = 3 # Wednesday
  start_hour_utc = 12
}
```

## Import

```sh
terraform import clickhouse_service_upgrade_window.example <service_id>
```
