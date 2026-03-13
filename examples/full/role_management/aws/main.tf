variable "organization_id" {
  type = string
}

variable "token_key" {
  type      = string
  sensitive = true
}

variable "token_secret" {
  type      = string
  sensitive = true
}

variable "suffix" {
  type    = string
  default = ""
}

# Look up the ID of the API key used by the provider
data "clickhouse_api_key_id" "current" {}

# Look up the built-in Member system role by name to avoid hardcoding its UUID
data "clickhouse_role" "member" {
  name = "Member"
}

# Look up a user by email address
/*
data "clickhouse_user" "alice" {
  email = "example@email.com"
}
*/

# Assign both a user and the provider's API key to the Member system role.
# A single resource manages all actors for the role, avoiding race conditions.
resource "clickhouse_role_assignment" "member" {
  role_id = data.clickhouse_role.member.id

  // user_ids    = [data.clickhouse_user.alice.id]
  api_key_ids = [data.clickhouse_api_key_id.current.id]
}

# A minimal custom role with no actors or policies
resource "clickhouse_role" "minimal" {
  name = "tf-test-minimal${var.suffix}"
}

# A custom role with actors managed via a role assignment resource
resource "clickhouse_role" "with_actors" {
  name = "tf-test-with-actors${var.suffix}"
}

resource "clickhouse_role_assignment" "with_actors" {
  role_id = clickhouse_role.with_actors.id

  // user_ids    = [data.clickhouse_user.alice.id]
  api_key_ids = [data.clickhouse_api_key_id.current.id]
}

# A role with a simple ALLOW policy
resource "clickhouse_role" "with_policy" {
  name = "tf-test-with-policy${var.suffix}"

  policies = [
    {
      effect      = "ALLOW"
      permissions = ["control-plane:service:view"]
      resources   = ["instance/*"]
    }
  ]
}

resource "clickhouse_role_assignment" "with_policy" {
  role_id = clickhouse_role.with_policy.id

  user_ids    = [data.clickhouse_user.alice.id]
  api_key_ids = [data.clickhouse_api_key_id.current.id]
}

# A role that grants read-only access to organization-level billing information.
# Organization permissions are scoped to "organization/<id>" rather than "instance/*".
resource "clickhouse_role" "billing_viewer" {
  name = "tf-test-billing-viewer${var.suffix}"

  policies = [
    {
      effect = "ALLOW"
      permissions = [
        "control-plane:organization:view-billing",
      ]
      resources = ["organization/${var.organization_id}"]
    },
  ]
}

# A role with mixed organization and service permissions, including a DENY policy.
# The operator can manage services, but billing and API key
# management are explicitly denied at the organization level.
resource "clickhouse_role" "restricted_operator" {
  name = "tf-test-restricted-operator${var.suffix}"

  policies = [
    {
      effect = "ALLOW"
      permissions = [
        "control-plane:service:view",
        "control-plane:service:manage",
        "control-plane:service:view-backups",
        "control-plane:service:manage-backups",
      ]
      resources = ["instance/*"]
    },
    {
      effect = "DENY"
      permissions = [
        "control-plane:organization:manage-billing",
        "control-plane:organization:view-billing",
        "control-plane:organization:create-api-keys",
        "control-plane:organization:delete-api-keys",
      ]
      resources = ["organization/${var.organization_id}"]
    },
  ]
}

# A role with tags on the policy (sql-console access)
resource "clickhouse_role" "sql_console" {
  name = "tf-test-sql-console${var.suffix}"

  policies = [
    {
      effect      = "ALLOW"
      permissions = ["sql-console:database:access"]
      resources   = ["instance/*"]
      tags = {
        role = "sql-console-readonly" # or "sql-console-admin" for full access
      }
    }
  ]
}

output "minimal_role_id" {
  value = clickhouse_role.minimal.id
}

output "with_actors_role_id" {
  value = clickhouse_role.with_actors.id
}

output "with_policy_role_id" {
  value = clickhouse_role.with_policy.id
}

output "billing_viewer_role_id" {
  value = clickhouse_role.billing_viewer.id
}

output "sql_console_role_id" {
  value = clickhouse_role.sql_console.id
}

output "restricted_operator_role_id" {
  value = clickhouse_role.restricted_operator.id
}
