resource "clickhouse_service" "svc" {
  ...
}

resource "clickhouse_service_scheduled_scaling" "example" {
  service_id = clickhouse_service.svc.id

  entries = [
    {
      # Horizontal: the replica count scales across the band at a fixed per-replica memory while active.
      name                  = "Business hours"
      weekdays              = [1, 2, 3, 4, 5]
      start_hour_utc        = 8
      end_hour_utc          = 18
      autoscaling_mode      = "horizontal"
      min_replicas          = 3
      max_replicas          = 6
      min_replica_memory_gb = 16
      max_replica_memory_gb = 16
      idle_scaling          = false
    },
    {
      # Vertical (the default when autoscaling_mode is omitted): a fixed replica count (min == max).
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

resource "clickhouse_service" "gen2_svc" {
  profile = "v2-default"
  ...
}

# On a Gen 2 service (profile = "v2-default") every entry is a fixed size and can't idle:
# equal min/max memory, equal min/max replicas and idle_scaling = false.
resource "clickhouse_service_scheduled_scaling" "gen2_example" {
  service_id = clickhouse_service.gen2_svc.id

  entries = [
    {
      name                  = "Business hours"
      weekdays              = [1, 2, 3, 4, 5]
      start_hour_utc        = 8
      end_hour_utc          = 18
      min_replicas          = 4
      max_replicas          = 4
      min_replica_memory_gb = 32
      max_replica_memory_gb = 32
      idle_scaling          = false
    },
  ]
}
