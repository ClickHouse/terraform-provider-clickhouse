You can use the *clickhouse_role* resource to manage custom RBAC roles in ClickHouse Cloud.

~> **Note:** This resource is in alpha. To assign actors (users or API keys) to a role, use the `clickhouse_role_assignment` resource.

## Example Usage

```hcl
resource "clickhouse_role" "example" {
  name = "my-custom-role"

  policies = [
    # Organization-level permission
    {
      effect  = "ALLOW"
      permissions = ["control-plane:organization:create-api-keys"]
      resources   = ["organization/<org-id>"]
    },
    # Service-level permission scoped to a specific service
    {
      effect  = "ALLOW"
      permissions = ["control-plane:service:view-backups"]
      resources   = ["instance/<service-id>"]
    },
    # SQL console passwordless DB access
    {
      effect  = "ALLOW"
      permissions = ["sql-console:database:access"]
      resources   = ["instance/<service-id>"]
      tags = {
        role = "sql-console-readonly"
      }
    },
  ]
}

data "clickhouse_user" "alice" {
  email = "alice@example.com"
}

resource "clickhouse_role_assignment" "example" {
  role_id  = clickhouse_role.example.id
  user_ids = [data.clickhouse_user.alice.id]
}
```
