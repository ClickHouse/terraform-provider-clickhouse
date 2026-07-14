Use this data source to look up a single RBAC role by ID or name. Exactly one of `id` or `name` must be set.

## Example Usage

```hcl
# Look up a system role by name
data "clickhouse_role" "admin" {
  name = "Admin"
}

# Look up a custom role by ID
data "clickhouse_role" "custom" {
  id = "47f0b035-d6ca-4600-8c69-9955551857e5"
}
```
