Use the *clickhouse_invitation* resource to invite a new user to your ClickHouse Cloud organization and assign their initial roles. A member only exists in the organization after they accept an invitation and log in for the first time, so this resource is how you onboard a brand-new user declaratively.

Invitations are immutable. Changing `email`, `assigned_role_ids`, or `role` revokes the existing invitation and sends a new one (destroy + recreate).

~> **Note:** This resource governs the user's roles only *at accept time*. Manage a member's roles thereafter with the `clickhouse_role_assignment` resource, using the `user_id` from the `clickhouse_user` data source. Deleting a `clickhouse_invitation` revokes a still-pending invite; it does **not** remove a user who has already accepted.

When an invited user accepts, the invitation is consumed server-side. On the next refresh the provider detects that the email now belongs to a member and keeps the resource in state with no diff, so onboarding is not repeated.

Use `assigned_role_ids` to assign roles by ID. Look up system role IDs with the `clickhouse_role` data source, or reference a `clickhouse_role` resource for custom roles. The `role` attribute is a deprecated legacy alternative and conflicts with `assigned_role_ids`.

## Example Usage

```hcl
data "clickhouse_role" "developer" {
  name = "Developer"
}

resource "clickhouse_invitation" "alice" {
  email             = "alice@example.com"
  assigned_role_ids = [data.clickhouse_role.developer.id]
}
```
