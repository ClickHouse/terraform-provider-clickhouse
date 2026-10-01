You can use the *clickhouse_service_clickhouse_settings* resource to configure ClickHouse settings (for example `compatibility` or `max_query_size`) on a ClickHouse Cloud service.

~> **Note:** This resource is in beta and uses a beta API endpoint whose contract may change. The API key needs the `control-plane:service:manage` permission.

## Managed settings

Only the settings listed in `settings` are managed. Settings configured on the service outside of Terraform are left untouched and do not show up as drift. Removing a setting from `settings`, or destroying the resource, resets it to the platform default.

Use the [settings schema endpoint](https://clickhouse.com/docs/products/cloud/api-reference/service/service-clickhouse-settings-schema-get) to discover which settings are configurable on a service and their types.

## Background merges

Set `merges_enabled = false` to stop the service from assigning background merges and mutations, for example on a compute group that should only serve reads. This maps to the `shared_merge_tree_disable_merges_and_mutations_assignment` setting, which cannot also be set through `settings`. Changing it triggers a rolling restart of the service. Leave it unset to keep the platform default.

## Values

Values are always written as strings in Terraform. The provider sends them to the API in the setting's native type: settings the schema types as `string` are sent as strings, and other values that parse as an integer are sent as integers.

## Disruptive settings

Some settings, such as `compatibility`, can cause query failures if changed without testing. The API returns a warning for these, which the provider surfaces as a Terraform warning. Server-level settings (for example `keep_alive_timeout`) trigger a rolling restart of the service when changed or reset.

## Import

```sh
terraform import clickhouse_service_clickhouse_settings.example <service_id>
```

Import adopts every setting currently configured on the service. Settings with a dedicated attribute, such as `merges_enabled`, are imported into that attribute. Keep everything in your configuration, otherwise the next apply resets the settings you leave out.

## Example Usage

```hcl
resource "clickhouse_service_clickhouse_settings" "example" {
  service_id = clickhouse_service.example.id

  settings = {
    compatibility  = "26.2"
    max_query_size = "262144"
  }
}

resource "clickhouse_service_clickhouse_settings" "read_only" {
  service_id     = clickhouse_service.read_only.id
  merges_enabled = false
}
```
