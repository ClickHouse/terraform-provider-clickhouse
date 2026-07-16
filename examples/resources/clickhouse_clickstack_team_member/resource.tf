# Assign a role to a team member. If the email already has an account the role
# is assigned immediately; otherwise a pending invitation is created and an
# invite URL is exposed via the (sensitive) `invite_url` attribute.
data "clickhouse_clickstack_role" "member" {
  name = "Member"
}

resource "clickhouse_clickstack_team_member" "alice" {
  email   = "alice@example.com"
  name    = "Alice"
  role_id = data.clickhouse_clickstack_role.member.id
}

output "alice_invite_url" {
  value     = clickhouse_clickstack_team_member.alice.invite_url
  sensitive = true
}
