# Manage settings for the existing team. This resource adopts the team on
# create; destroying it leaves the team and its settings untouched.
data "clickhouse_clickstack_role" "read_only" {
  name = "ReadOnly"
}

resource "clickhouse_clickstack_team" "main" {
  default_user_role_id = data.clickhouse_clickstack_role.read_only.id
}
