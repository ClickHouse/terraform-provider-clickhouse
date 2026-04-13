data "clickhouse_role" "member" {
  name = "Member"
}

data "clickhouse_user" "alice" {
  email = "alice@example.com"
}

data "clickhouse_api_key_id" "current" {}

resource "clickhouse_role_assignment" "member" {
  role_id = data.clickhouse_role.member.id

  user_ids    = [data.clickhouse_user.alice.id]
  api_key_ids = [data.clickhouse_api_key_id.current.id]
}
