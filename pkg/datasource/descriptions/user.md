Use this data source to look up an organization member by email or user ID. Exactly one of `email` or `id` must be set.

## Example Usage

```hcl
data "clickhouse_user" "alice" {
  email = "alice@example.com"
}

data "clickhouse_role" "member" {
  name = "Member"
}

resource "clickhouse_role_assignment" "alice_member" {
  role_id  = data.clickhouse_role.member.id
  user_ids = [data.clickhouse_user.alice.id]
}
```
