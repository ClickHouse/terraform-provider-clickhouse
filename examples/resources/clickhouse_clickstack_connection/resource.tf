resource "clickhouse_clickstack_connection" "main" {
  name     = "Production ClickHouse"
  host     = "https://clickhouse.example.com:8443"
  username = "default"
  password = var.clickhouse_password

  # Optional: proxy PromQL queries to a Prometheus-compatible endpoint.
  # prometheus_endpoint = "http://prometheus:9090"
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
