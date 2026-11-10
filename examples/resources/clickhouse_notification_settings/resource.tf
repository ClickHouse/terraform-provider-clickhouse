data "clickhouse_user" "alice" {
  email = "alice@example.com"
}

resource "clickhouse_notification_settings" "alice_services" {
  user_id  = data.clickhouse_user.alice.id
  channels = ["ui", "email"]

  rules = [
    # Explicitly off for this one service, even though the platform default is on.
    {
      effect        = "disabled"
      notifications = ["service_metrics_too_many_parts"]
      resources     = ["xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"]
    },
    # Hands the setting back to the platform default, whoever set it.
    {
      effect        = "inherit"
      notifications = ["service_backup_failed"]
      resources     = ["xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"]
    },
  ]
}
