# ClickStack on ClickHouse Cloud e2e test.
#
# This example runs against a long-lived Cloud service that already has managed
# ClickStack onboarded. It does not create the service: onboarding is a console
# step the provider cannot do, and `clickstack_service_id` must be known at
# provider-configure time, which rules out creating the service in the same
# apply. See the README for the one-time setup.
#
# Connections are not managed here. On Cloud the connections endpoint is not
# exposed: the platform creates one connection per service, pointing at itself,
# and its id comes in as a variable.
#
# There is no team id to supply: on Cloud a service *is* the ClickStack team,
# so the client never sends the x-hdx-team header and rejects a `team` attribute
# outright (internal/service/clickstack/client/client.go).
#
# Teams are deliberately absent: the ClickStack team endpoints 404 on Cloud by
# design, so clickhouse_clickstack_{team,team_member} are self-hosted only.
# Roles are not — ClickStack RBAC is its own system, separate from Cloud's, and
# serves full CRUD here.

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

variable "clickstack_service_id" {
  type        = string
  description = "ID of the long-lived Cloud service running managed ClickStack."
}

variable "connection_id" {
  type        = string
  description = "ID of the service's platform-created ClickStack connection."
}

# Appended to every ClickStack object name. Runs share one service, so this is
# what keeps concurrent runs from colliding — give each run its own value.
variable "suffix" {
  type    = string
  default = ""
}

# The e2e run applies twice: once with this false, once true. Without it the run
# only ever covers create and destroy, and every in-place update path (the API's
# PUT routes) goes untested. The dashboard changes are the ones that matter:
# renaming a tile exercises the tile-id carry-forward that keeps UI-created tile
# alerts bound, and adding a filter exercises the filter-id minting the Cloud API
# requires on update but rejects on create.
variable "update_pass" {
  type    = bool
  default = false
}

resource "clickhouse_clickstack_source" "logs" {
  name          = "tf-e2e-logs${var.suffix}"
  kind          = "log"
  connection_id = var.connection_id

  from = {
    database_name = "default"
    table_name    = "otel_logs"
  }

  timestamp_value_expression      = "TimestampTime"
  default_table_select_expression = "Timestamp, ServiceName, SeverityText, Body"

  service_name_expression        = "ServiceName"
  severity_text_expression       = "SeverityText"
  body_expression                = "Body"
  resource_attributes_expression = "ResourceAttributes"
  event_attributes_expression    = "LogAttributes"

  # Nested list blocks, so refresh round-trips the nested mappers rather than
  # scalar fields only.
  query_settings = var.update_pass ? [
    { setting = "max_threads", value = "8" },
    { setting = "max_execution_time", value = "30" },
    ] : [
    { setting = "max_threads", value = "4" },
  ]

  highlighted_row_attribute_expressions = [
    { sql_expression = "ServiceName", alias = "Service" },
  ]
}

resource "clickhouse_clickstack_saved_search" "errors" {
  name           = "tf-e2e-errors${var.suffix}"
  source_id      = clickhouse_clickstack_source.logs.id
  select         = "Timestamp, ServiceName, Body"
  where          = var.update_pass ? "SeverityText IN ('ERROR', 'FATAL')" : "SeverityText = 'ERROR'"
  where_language = "sql"
  tags           = ["terraform", "e2e"]
}

resource "clickhouse_clickstack_webhook" "alerts" {
  name    = "tf-e2e${var.suffix}"
  service = "generic"
  url     = "https://example.com/hooks/tf-e2e"
  body    = jsonencode({ text = "tf e2e" })
}

resource "clickhouse_clickstack_alert" "too_many_errors" {
  saved_search_id = clickhouse_clickstack_saved_search.errors.id

  channel = {
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.alerts.id
  }

  threshold      = var.update_pass ? 200 : 100
  threshold_type = "above"

  # Longest supported window, deliberately. CRUD coverage is identical at any
  # interval, and the alert costs 1 evaluation/day instead of 288 in the window
  # between apply and destroy.
  interval = "1d"

  name    = "tf-e2e${var.suffix}"
  message = "Error volume exceeded threshold"
}

# Custom RBAC role. ClickStack RBAC is its own system: it governs ClickStack
# objects, and a role created here does not appear in Cloud's role list, so
# clickhouse_role is not a substitute. Covered because nothing else exercises the
# permission round-trip against Cloud — the API auto-injects a `read Connection`
# rule the config never asked for, and the resource has to filter it back out or
# every plan drifts.
resource "clickhouse_clickstack_role" "readonly" {
  name        = "tf-e2e-readonly${var.suffix}"
  description = var.update_pass ? "Read-only, plus alerts" : "Read-only e2e role"

  permissions = var.update_pass ? [
    { action = "read", subject = "Dashboard" },
    { action = "read", subject = "Source" },
    { action = "manage", subject = "Alert" },
    ] : [
    { action = "read", subject = "Dashboard" },
    { action = "read", subject = "Source" },
  ]
}

# Covers both tile shapes, and with them POST /dashboards/validate: one tile
# driven by a ClickStack source, one by a raw SQL query on the connection.
resource "clickhouse_clickstack_dashboard" "e2e" {
  dashboard_json = jsonencode({
    name = "tf-e2e${var.suffix}"
    tags = ["terraform", "e2e"]

    # Filters are absent on the create pass and present on the update pass. The
    # asymmetry is the point: POST rejects a filter carrying an id, PUT requires
    # one on every filter, so the provider has to mint them on update.
    filters = var.update_pass ? [
      {
        type       = "QUERY_EXPRESSION"
        name       = "errors-only"
        expression = "SeverityText = 'ERROR'"
        sourceId   = clickhouse_clickstack_source.logs.id
      }
    ] : []

    tiles = [
      {
        # Renamed on the update pass. Tile-id carry-forward matches on name, so
        # a rename forces the positional fallback — the path that would silently
        # drop a UI-created tile alert if it regressed.
        name = var.update_pass ? "Log volume (renamed)" : "Log volume"
        x    = 0
        y    = 0
        w    = 6
        h    = 3
        config = {
          displayType   = "line"
          sourceId      = clickhouse_clickstack_source.logs.id
          where         = ""
          whereLanguage = "sql"
          select = [{
            aggFn           = "count"
            valueExpression = ""
            alias           = "Logs"
          }]
        }
      },
      {
        name = "Top services"
        x    = 6
        y    = 0
        w    = 6
        h    = 3
        config = {
          configType   = "sql"
          displayType  = "table"
          connectionId = var.connection_id
          sqlTemplate  = "SELECT ServiceName, count() AS logs FROM default.otel_logs GROUP BY ServiceName ORDER BY logs DESC LIMIT 20"
        }
      }
    ]
  })
}

output "source_id" {
  value = clickhouse_clickstack_source.logs.id
}

output "saved_search_id" {
  value = clickhouse_clickstack_saved_search.errors.id
}

output "webhook_id" {
  value = clickhouse_clickstack_webhook.alerts.id
}

output "alert_id" {
  value = clickhouse_clickstack_alert.too_many_errors.id
}

output "dashboard_id" {
  value = clickhouse_clickstack_dashboard.e2e.id
}

output "role_id" {
  value = clickhouse_clickstack_role.readonly.id
}
