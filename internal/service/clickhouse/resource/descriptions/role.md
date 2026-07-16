You can use the *clickhouse_role* resource to manage custom RBAC roles in ClickHouse Cloud.

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

## Permission reconciliation

The provider only tracks the permissions you declare in configuration. This has two consequences:

- The backend may auto-grant additional permissions as a side effect of a declared one (for example, granting `control-plane:service:manage` may also grant a related permission). Those extra permissions are intentionally not recorded in state.
- Because of this, a permission added to a policy outside of Terraform (e.g. via the console) is **not** reported as drift and will not be removed on the next apply. To manage a permission with Terraform, add it to the `permissions` list.

A permission you declared that the backend actually removed is still detected as drift and re-added on the next apply.
