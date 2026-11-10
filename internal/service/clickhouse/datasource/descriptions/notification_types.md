~> **Note:** This data source is in beta.

Use the *clickhouse_notification_types* data source to list the notification types the organization can receive. It is the registry a `clickhouse_notification_settings` rule names types from, read at plan time, so a type added to ClickHouse Cloud needs no provider release.

Types the organization is not entitled to are omitted. A type the API key lacks the permission for is still listed, with `is_available` set to false.

## Example Usage

```hcl
data "clickhouse_notification_types" "all" {}

# Only a configurable type can carry a rule; a rule naming any other is rejected.
output "configurable_service_notifications" {
  value = [
    for t in data.clickhouse_notification_types.all.types : t.name
    if t.is_configurable && t.resource_type == "service"
  ]
}
```

Drive a rule set from the registry rather than a hard-coded list:

```hcl
resource "clickhouse_notification_settings" "alice_critical" {
  user_id  = data.clickhouse_user.alice.id
  channels = ["email"]

  rules = [
    {
      effect = "enabled"
      notifications = [
        for t in data.clickhouse_notification_types.all.types : t.name
        if t.severity == "critical" && t.is_configurable && t.resource_type == "service"
      ]
      resources = [clickhouse_service.prod.id]
    },
  ]
}
```
