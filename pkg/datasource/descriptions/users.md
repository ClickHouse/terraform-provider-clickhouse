Use this data source to list all members of the organization. Unlike the singular `clickhouse_user` data source, this one never errors when a particular user is absent — a missing user is simply not present in the returned `users` list.

This is useful when you need to reference a user that may not exist yet (for example, someone who has only been invited via `clickhouse_invitation` and has not yet accepted). Filter the list in HCL and guard on whether a match was found.

~> **Note:** A user who has only been invited (invitation pending) is not yet an organization member and will **not** appear in this list. Members exist only after the invited user accepts and logs in for the first time.

## Example Usage

```hcl
data "clickhouse_users" "all" {}

data "clickhouse_role" "member" {
  name = "Member"
}

locals {
  # Zero or one element; null if the user has not accepted their invitation yet.
  alice = one([for u in data.clickhouse_users.all.users : u if u.email == "alice@example.com"])
}

# Only assign the role once the user actually exists as a member.
resource "clickhouse_role_assignment" "alice_member" {
  count = local.alice == null ? 0 : 1

  role_id  = data.clickhouse_role.member.id
  user_ids = [local.alice.id]
}
```
