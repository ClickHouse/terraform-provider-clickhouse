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

variable "service_name" {
  type    = string
  default = "my-postgres-e2e"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

# The shared e2e harness generates names like "[e2e]-postgres-...": sanitize to
# the Postgres instance-name charset (lowercase, alnum + hyphen).
locals {
  pg_name = lower(replace(replace(replace(replace(var.service_name, "[", ""), "]", ""), ".", "-"), " ", "-"))
}

# Primary: exercises explicit version, HA, runtime config (pg_config /
# pgbouncer_config), tags, and a user-managed password.
resource "clickhouse_postgres_service" "primary" {
  name             = local.pg_name
  cloud_provider   = "aws"
  region           = var.region
  size             = "c6gd.large"
  postgres_version = "18"
  ha_type          = "async"

  password = "TerraformE2E123"

  pg_config = {
    max_connections = "200"
  }

  pgbouncer_config = {
    default_pool_size = "25"
  }

  tags = {
    environment = "e2e"
    managed_by  = "terraform"
  }
}

# Read replica of the primary (inherits the superuser, so no password here).
resource "clickhouse_postgres_service" "replica" {
  name            = "${local.pg_name}-replica"
  cloud_provider  = "aws"
  region          = var.region
  size            = "c6gd.large"
  read_replica_of = clickhouse_postgres_service.primary.id
}

# All three data sources, against the primary.
data "clickhouse_postgres_service" "by_id" {
  id = clickhouse_postgres_service.primary.id
}

data "clickhouse_postgres_services" "all" {}

data "clickhouse_postgres_service_ca_certificates" "certs" {
  service_id = clickhouse_postgres_service.primary.id
}

output "primary_id" {
  value = clickhouse_postgres_service.primary.id
}

output "replica_id" {
  value = clickhouse_postgres_service.replica.id
}

output "primary_is_primary" {
  value = clickhouse_postgres_service.primary.is_primary
}

output "replica_is_primary" {
  value = clickhouse_postgres_service.replica.is_primary
}

output "ca_certificate_present" {
  value = length(data.clickhouse_postgres_service_ca_certificates.certs.certificate) > 0
}

output "services_count" {
  value = length(data.clickhouse_postgres_services.all.services)
}
