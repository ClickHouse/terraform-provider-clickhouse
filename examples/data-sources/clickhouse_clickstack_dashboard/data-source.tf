# Look up an existing dashboard by its ID to reference its configuration, for
# example when sharing dashboard data across modules or validating current state.
data "clickhouse_clickstack_dashboard" "example" {
  id = var.dashboard_id
}

output "dashboard_json" {
  value = data.clickhouse_clickstack_dashboard.example.dashboard_json
}

variable "dashboard_id" { type = string }
