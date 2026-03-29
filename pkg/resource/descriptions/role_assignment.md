Use the *clickhouse_role_assignment* resource to assign actors (users and/or API keys) to a role. Works for both system roles and custom roles.

One resource manages all actors for a given role. Use `user_ids` to assign users, `api_key_ids` to assign API keys, or both. Only one `clickhouse_role_assignment` per role is needed.

~> **Note:** This resource is in alpha. On delete, all actors are removed from the role.

To look up the ID of a system role by name, use the `clickhouse_role` data source.
To look up a user ID by email, use the `clickhouse_user` data source.

## Example Usage

```hcl
data "clickhouse_role" "member" {
  name = "Member"
}

data "clickhouse_user" "alice" {
  email = "alice@example.com"
}

data "clickhouse_user" "bob" {
  email = "bob@example.com"
}

data "clickhouse_api_key_id" "current" {}

resource "clickhouse_role_assignment" "member" {
  role_id = data.clickhouse_role.member.id

  user_ids    = [data.clickhouse_user.alice.id, data.clickhouse_user.bob.id]
  api_key_ids = [data.clickhouse_api_key_id.current.id]
}
```
