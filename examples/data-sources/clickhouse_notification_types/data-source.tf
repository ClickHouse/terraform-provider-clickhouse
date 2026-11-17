data "clickhouse_notification_types" "all" {}

# Only a configurable type can carry a rule; a rule naming any other is rejected.
output "configurable_service_notifications" {
  value = [
    for t in data.clickhouse_notification_types.all.types : t.name
    if t.is_configurable && t.resource_type == "service"
  ]
}
