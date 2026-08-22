You can use the *clickhouse_service_upgrade_window* resource to pin the weekly upgrade window for a ClickHouse Cloud service.

When configured, the data plane only attempts service upgrades during the declared window. The window is a single weekly recurrence: a `weekday` plus a `start_hour_utc`. The window currently lasts 6 hours from `start_hour_utc`; `duration` is returned by the API and exposed as a read-only attribute.

~> **Note:** This resource is in beta. Setting or updating an upgrade window requires the `canUseScheduledUpgrades` entitlement — reach out to ClickHouse support if the API returns `403 FORBIDDEN`. Deleting an upgrade window is allowed even after the entitlement is lost, so existing windows can always be cleared.

## Primary services only

The upgrade window can only be set on primary services. Secondary services inherit the window from their primary. Applying this resource against a secondary service returns a `400` with the message `cannot set upgrade window on a secondary service`, surfaced by the provider as `Upgrade windows can only be set on primary services`.

## Allowed values

- `weekday`: `0` (Sunday) – `6` (Saturday)
- `start_hour_utc`: one of `0`, `6`, `12`, `18`

## Best-effort overwrite protection

`Create` performs a `GET` before issuing the `PUT` so that an existing window (e.g. one configured out-of-band) surfaces a "please import" diagnostic instead of being silently overwritten. The check is best-effort: a window created between this resource's `GET` and `PUT` (e.g. by another tool, or another Terraform apply) will still be overwritten by `PUT`.

## Import

```sh
terraform import clickhouse_service_upgrade_window.example <service_id>
```

Import is refused with a clear diagnostic when the supplied service ID does not exist or points at a secondary service. Importing a primary service that does not yet have an upgrade window configured succeeds, but the subsequent Read returns `404` and removes the resource from state — leaving the next plan proposing a fresh create. Confirm the service ID is correct before importing.

## Example Usage

```hcl
resource "clickhouse_service_upgrade_window" "example" {
  service_id     = clickhouse_service.example.id
  weekday        = 3 # Wednesday
  start_hour_utc = 12
}
```
