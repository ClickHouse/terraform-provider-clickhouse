# Look up a predefined role (Admin, Member, ReadOnly) to reference its ID, for
# example when assigning a default new-user role or a team member's role.
data "clickhouse_clickstack_role" "member" {
  name = "Member"
}

output "member_role_id" {
  value = data.clickhouse_clickstack_role.member.id
}
