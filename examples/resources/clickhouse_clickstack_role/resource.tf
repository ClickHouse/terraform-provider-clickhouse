resource "clickhouse_clickstack_role" "analyst" {
  name        = "Analyst"
  description = "Read-only access to dashboards and searches"

  permissions = [
    {
      action  = "read"
      subject = "Dashboard"
    },
    {
      action  = "read"
      subject = "SavedSearch"
    },
  ]
}

# A permission can be restricted with a JSON-encoded conditions object, denied
# with `inverted`, or scoped to the ClickHouse (data RBAC) integration.
resource "clickhouse_clickstack_role" "restricted_editor" {
  name = "Restricted Editor"

  permissions = [
    {
      action  = "manage"
      subject = "Dashboard"
    },
    {
      action     = "read"
      subject    = "Source"
      conditions = jsonencode({ kind = "log" })
    },
    {
      action      = "read"
      subject     = "all"
      integration = "clickhouse"
    },
  ]
}
