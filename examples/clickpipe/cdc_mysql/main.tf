variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse Cloud service ID"
}

variable "mysql_host" {
  description = "MySQL host address"
}

variable "mysql_port" {
  description = "MySQL port"
  default     = 3306
}

variable "mysql_username" {
  description = "MySQL username"
  sensitive   = true
}

variable "mysql_password" {
  description = "MySQL password"
  sensitive   = true
}

variable "mysql_schema" {
  description = "MySQL schema (database) name"
  default     = "mydb"
}

variable "source_table" {
  description = "Source table name in MySQL"
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

# MySQL CDC ClickPipe
resource "clickhouse_clickpipe" "mysql_cdc" {
  name       = "MySQL CDC ClickPipe"
  service_id = var.service_id

  # Ensure CDC infrastructure is created first
  depends_on = [clickhouse_clickpipe_cdc_infrastructure.infra]

  source = {
    mysql = {
      host = var.mysql_host
      port = var.mysql_port

      credentials = {
        username = var.mysql_username
        password = var.mysql_password
      }

      settings = {
        replication_mode      = "cdc" # Options: "cdc", "snapshot", "cdc_only"
        replication_mechanism = "AUTO" # Options: "AUTO", "GTID", "FILE_POS"
        # Optional: Uncomment to customize
        # use_compression = true
        # sync_interval_seconds = 60
        # pull_batch_size = 1000
        # allow_nullable_columns = true
      }

      table_mappings = [
        {
          source_schema_name = var.mysql_schema
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
    # Note: For MySQL CDC, managed_table is always false
    # Tables are managed via table_mappings
  }
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.mysql_cdc.id
}

output "clickpipe_state" {
  value = clickhouse_clickpipe.mysql_cdc.state
}

output "cdc_infrastructure_cpu" {
  value = clickhouse_clickpipe_cdc_infrastructure.infra.replica_cpu_millicores
}

output "cdc_infrastructure_memory" {
  value = clickhouse_clickpipe_cdc_infrastructure.infra.replica_memory_gb
}
