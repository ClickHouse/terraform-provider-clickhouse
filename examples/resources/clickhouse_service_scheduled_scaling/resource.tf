resource "clickhouse_service_scheduled_scaling" "example" {
  service_id = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"

  entries = [
    {
      name           = "Business hours"
      weekdays       = [1, 2, 3, 4, 5]
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
      end_hour_utc         = 6
      min_replicas         = 1
      max_replicas         = 1
      idle_scaling         = true
      idle_timeout_minutes = 5
    },
  ]
}
