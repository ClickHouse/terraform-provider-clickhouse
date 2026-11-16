resource "clickhouse_clickstack_connection" "main" {
  name     = "Production ClickHouse"
  host     = "https://clickhouse.example.com:8443"
  username = "default"
  password = var.clickhouse_password
}

# A Prometheus-compatible endpoint (e.g. Prometheus or Thanos). PromQL sources
# on this connection proxy their queries to host. Username and password are
# ignored; put any Basic auth credentials in the host URL.
resource "clickhouse_clickstack_connection" "prometheus" {
  name                   = "Prometheus"
  host                   = "http://prometheus:9090"
  username               = ""
  is_prometheus_endpoint = true
}

# Managing connections across multiple teams from one configuration. (Enterprise Only)
#
# A single provider (one API key) can manage connections in every team the key
# has access to by setting `team` per resource — no need for multiple aliased
# provider blocks. `team` is sent as the x-hdx-team header and is only honored
# by multi-team (EE) deployments; single-team (OSS) deployments ignore it, so
# the same configuration is portable across both.
locals {
  connections = {
    "platform/prod"  = { team = "65f0c0ffeecafef00dba5e01", name = "Platform prod", host = "https://a:8443", username = "default" }
    "platform/stage" = { team = "65f0c0ffeecafef00dba5e01", name = "Platform stage", host = "https://s:8443", username = "default" }
    "growth/prod"    = { team = "65f0c0ffeecafef00dba5e02", name = "Growth prod", host = "https://g:8443", username = "default" }
  }
}

resource "clickhouse_clickstack_connection" "by_team" {
  for_each = local.connections

  team     = each.value.team
  name     = each.value.name
  host     = each.value.host
  username = each.value.username
  password = var.clickhouse_passwords[each.key]
}

# On ClickHouse Cloud the connections endpoint is not exposed, so this resource
# cannot create, change, or delete connections there. Cloud gives each service a
# single connection to itself; read its id off an existing source and reference
# that instead.
resource "clickhouse_clickstack_source" "logs" {
  name          = "Logs"
  kind          = "log"
  connection_id = "507f1f77bcf86cd799439012"

  from = {
    database_name = "otel"
    table_name    = "otel_logs"
  }
  timestamp_value_expression = "Timestamp"
}
