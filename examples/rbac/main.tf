variable "organization_id" {
  type = string
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

variable "service_name" {
  type = string
  default = "My Terraform Service"
}

variable "region" {
  type = string
  default = "us-east-2"
}

variable "release_channel" {
  type = string
  default = "default"
  validation {
    condition     = var.release_channel == "default" || var.release_channel == "fast"
    error_message = "Release channel can be either 'default' or 'fast'."
  }
}

data "clickhouse_api_key_id" "self" {
}

resource "clickhouse_service" "service" {
  name                      = var.service_name
  cloud_provider            = "aws"
  region                    = var.region
  release_channel           = var.release_channel
  idle_scaling              = true
  idle_timeout_minutes      = 5
  password_hash             = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0"
      description = "Anywhere"
    }
  ]

  # Required in order to create 'clickhouse_user', 'clickhouse_role' and 'clickhouse_grant*' resources below.
  query_api_endpoints = {
    api_key_ids = [
      data.clickhouse_api_key_id.self.id,
    ]
    roles = [
      "sql_console_admin"
    ]
    allowed_origins = null
  }

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120

  backup_configuration = {
    backup_period_in_hours           = 24
    backup_retention_period_in_hours = 24
    backup_start_time                = null
  }
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_user" "john" {
  service_id           = clickhouse_service.service.id
  name                 = "john"
  password_sha256_hash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" # sha256 of 'test'
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_role" "writer" {
  service_id           = clickhouse_service.service.id
  name                 = "writer"
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_grant_role" "writer_to_john" {
  service_id        = clickhouse_service.service.id
  role_name         = clickhouse_role.writer.name
  grantee_user_name = clickhouse_user.john.name
  admin_option      = false
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_role" "manager" {
  service_id           = clickhouse_service.service.id
  name                 = "manager"
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_grant_role" "writer_to_manager" {
  service_id        = clickhouse_service.service.id
  role_name         = clickhouse_role.writer.name
  grantee_role_name = clickhouse_role.manager.name
  admin_option      = false
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_grant_privilege" "grant_show_to_role" {
  service_id        = clickhouse_service.service.id
  privilege_name    = "SHOW"
  database_name     = "default"
  grantee_role_name = clickhouse_role.writer.name
  grant_option      = false
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_grant_privilege" "grant_global_privilege" {
  service_id        = clickhouse_service.service.id
  privilege_name    = "REMOTE"
  grantee_role_name = clickhouse_role.writer.name
  grant_option      = false
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_grant_privilege" "grant_dictget_to_role" {
  service_id        = clickhouse_service.service.id
  privilege_name    = "dictGet"
  database_name     = "default"
  grantee_role_name = clickhouse_role.writer.name
  grant_option      = false
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_grant_privilege" "grant_insert_on_table_to_user" {
  service_id        = clickhouse_service.service.id
  privilege_name    = "INSERT"
  database_name     = "default"
  table_name        = "tbl1"
  grantee_user_name = clickhouse_user.john.name
  grant_option      = true
}

# Requires 'query_api_endpoints' to be enabled in the service.
resource "clickhouse_grant_privilege" "grant_select_on_single_column_on_table_to_user" {
  service_id        = clickhouse_service.service.id
  privilege_name    = "SELECT"
  database_name     = "default"
  table_name        = "tbl1"
  column_name       = "count"
  grantee_user_name = clickhouse_user.john.name
  grant_option      = true
}
