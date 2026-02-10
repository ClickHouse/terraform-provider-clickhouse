resource "clickhouse_organization_settings" "this" {
  # Enable core dumps collection for services in the organization
  core_dumps_enabled = true
}
