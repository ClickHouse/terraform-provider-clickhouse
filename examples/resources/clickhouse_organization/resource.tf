resource "clickhouse_organization" "org_settings" {
  # Enable core dumps collection for services in the organization
  core_dumps_enabled = true
}
