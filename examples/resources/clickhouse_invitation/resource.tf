data "clickhouse_role" "developer" {
  name = "Developer"
}

resource "clickhouse_invitation" "alice" {
  email             = "alice@example.com"
  assigned_role_ids = [data.clickhouse_role.developer.id]
}
