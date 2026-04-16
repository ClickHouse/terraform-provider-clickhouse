resource "clickhouse_role" "example" {
  name = "my-custom-role"

  policies = [
    # Organization-level permission
    {
      effect      = "ALLOW"
      permissions = ["control-plane:organization:create-api-keys"]
      resources   = ["organization/<org-id>"]
    },
    # Service-level permission scoped to a specific service
    {
      effect      = "ALLOW"
      permissions = ["control-plane:service:view-backups"]
      resources   = ["instance/<service-id>"]
    },
  ]
}
