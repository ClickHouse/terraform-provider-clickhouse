You can use the *clickhouse_organization* resource to manage organization-level settings in ClickHouse Cloud.

~> **Note:** This resource manages settings for the organization configured in the provider. Only one instance of this resource should exist per organization.

## Example Usage

```terraform
resource "clickhouse_organization" "org_settings" {
  core_dumps_enabled = true
}
```
