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
  default = "europe-west4"
}

variable "release_channel" {
  type = string
  default = "default"
  validation {
    condition     = var.release_channel == "default" || var.release_channel == "fast"
    error_message = "Release channel can be either 'default' or 'fast'."
  }
}

resource "clickhouse_service" "service" {
  name                      = var.service_name
  cloud_provider            = "gcp"
  region                    = var.region
  tier                      = "production"
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

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120

  backup_configuration = {
    backup_period_in_hours           = 24
    backup_retention_period_in_hours = 24
    backup_start_time                = null
  }
}

resource "clickhouse_database" "mydatabase" {
  service_id = clickhouse_service.service.id
  name = "mydatabase"
  comment = "This is a test database"
}

resource "clickhouse_table" "mytable" {
  service_id = clickhouse_service.service.id
  database = clickhouse_database.mydatabase.name
  name = "mytable"
  comment = "This is a test table"
  order_by = "id"
  engine = {
    name = "SharedMergeTree"
    params = [
      "'/clickhouse/tables/{uuid}/{shard}'",
      "'{replica}'"
    ]
  }
  settings = {
    index_granularity = 2048
  }
  columns = {
    id = {
      type = "UInt8"
    }
    description = {
      type = "String"
      nullable = true
      comment = "The product description"
    }
    sector = {
      type = "String"
      ephemeral = true
    }
    function_default = {
      type = "String"
      default = "now()"
    }
    literal_default = {
      type = "String"
      default = "'Department 1'"
    }
    mat1 = {
      type = "String"
      materialized = "'Example literal'"
    }
    alias = {
      type = "DateTime"
      alias = "now()"
    }
  }
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "service_iam" {
  value = clickhouse_service.service.iam_role
}
