Use this data source to list all RBAC roles (both system and custom) in your ClickHouse Cloud organization.

## Example Usage

```hcl
data "clickhouse_roles" "all" {}

locals {
  admin_role_id = [for r in data.clickhouse_roles.all.roles : r.id if r.name == "Admin"][0]
}
```
