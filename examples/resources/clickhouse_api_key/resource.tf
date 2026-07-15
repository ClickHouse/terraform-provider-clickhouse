resource "clickhouse_api_key" "monitoring" {
  name  = "prometheus-scraper"
  state = "enabled"

  ip_access = [
    {
      source      = "10.0.0.0/8"
      description = "vpc"
    },
  ]
}

# Assign an RBAC role (system or custom) to the key from the role side.
resource "clickhouse_role_assignment" "monitoring" {
  role_id     = clickhouse_role.monitoring.id
  api_key_ids = [clickhouse_api_key.monitoring.id]
}
