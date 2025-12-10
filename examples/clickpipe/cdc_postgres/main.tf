variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse Cloud service ID"
}

variable "postgres_host" {
  description = "PostgreSQL host address"
}

variable "postgres_port" {
  description = "PostgreSQL port"
  default     = 5432
}

variable "postgres_database" {
  description = "PostgreSQL database name"
}

variable "postgres_username" {
  description = "PostgreSQL username"
  sensitive   = true
}

variable "postgres_password" {
  description = "PostgreSQL password"
  sensitive   = true
}

variable "postgres_schema" {
  description = "PostgreSQL schema name"
  default     = "public"
}

variable "source_table" {
  description = "Source table name in Postgres"
  default     = "users"
}

variable "target_table" {
  description = "Target table name in ClickHouse"
  default     = "users"
}

# CDC Infrastructure - manages shared compute resources for all CDC pipes
resource "clickhouse_clickpipe_cdc_infrastructure" "infra" {
  service_id             = var.service_id
  replica_cpu_millicores = 2000 # 2 CPU cores
  replica_memory_gb      = 8    # Must be 4x CPU cores (2 * 4 = 8)
}

# Postgres CDC ClickPipe
resource "clickhouse_clickpipe" "postgres_cdc" {
  name       = "Postgres CDC ClickPipe"
  service_id = var.service_id

  # Ensure CDC infrastructure is created first
  depends_on = [clickhouse_clickpipe_cdc_infrastructure.infra]

  source = {
    postgres = {
      host     = var.postgres_host
      port     = var.postgres_port
      database = var.postgres_database

      credentials = {
        username = var.postgres_username
        password = var.postgres_password
      }

      settings = {
        replication_mode = "cdc" # Options: "cdc", "snapshot", "cdc_only"
        # Optional: Uncomment to customize
        # publication_name = "my_publication"
        # sync_interval_seconds = 60
        # pull_batch_size = 1000
        # allow_nullable_columns = true
      }

      table_mappings = [
        {
          source_schema_name = var.postgres_schema
          source_table       = var.source_table
          target_table       = var.target_table
          # Optional: Specify table engine
          # table_engine = "ReplacingMergeTree"
          # Optional: Exclude columns
          # excluded_columns = ["sensitive_field"]
        }
      ]
    }
  }

  destination = {
    database = "default"
    # Note: For Postgres CDC, managed_table is always false
    # Tables are managed via table_mappings
  }
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.postgres_cdc.id
}

output "clickpipe_state" {
  value = clickhouse_clickpipe.postgres_cdc.state
}

output "cdc_infrastructure_cpu" {
  value = clickhouse_clickpipe_cdc_infrastructure.infra.replica_cpu_millicores
}

output "cdc_infrastructure_memory" {
  value = clickhouse_clickpipe_cdc_infrastructure.infra.replica_memory_gb
}
