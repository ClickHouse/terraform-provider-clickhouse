You can use the *clickhouse_service_clickhouse_settings* resource to configure ClickHouse settings (for example `compatibility` or `max_query_size`) on a ClickHouse Cloud service.

~> **Note:** This resource is in beta and uses a beta API endpoint whose contract may change. The API key needs the `control-plane:service:manage` permission.

## Managed settings

Only the settings listed in `settings` are managed. Settings configured on the service outside of Terraform are left untouched and do not show up as drift. Removing a setting from `settings`, or destroying the resource, resets it to the platform default.

Use the [settings schema endpoint](https://clickhouse.com/docs/products/cloud/api-reference/service/service-clickhouse-settings-schema-get) to discover which settings are configurable on a service and their types.

## Values

Values are always written as strings in Terraform. The provider sends settings the schema types as `integer` as integers, and all other settings as strings.

## Disruptive settings

Some settings, such as `compatibility`, can cause query failures if changed without testing. The API returns a warning for these, which the provider surfaces as a Terraform warning. Server-level settings (for example `keep_alive_timeout`) trigger a rolling restart of the service when changed or reset.

## Import

```sh
terraform import clickhouse_service_clickhouse_settings.example <service_id>
```

Import adopts every setting currently configured on the service. Keep them all in `settings`, otherwise the next apply resets the ones you leave out.

## Example Usage

```hcl
resource "clickhouse_service_clickhouse_settings" "example" {
  service_id = clickhouse_service.example.id

  settings = {
    compatibility  = "26.2"
    max_query_size = "262144"
  }
}
```
