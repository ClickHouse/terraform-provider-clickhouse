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
  default = "My ClickPipe Kafka Confluent Test"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "kafka_brokers" {
}

variable "kafka_topics" {
}

variable "kafka_username" {
  type        = string
  description = "Kafka username for SASL authentication"
  sensitive   = true
}

variable "kafka_password" {
  type        = string
  description = "Kafka password for SASL authentication"
  sensitive   = true
}

# Kinesis variables
variable "kinesis_stream_name" {
}

variable "kinesis_region" {
}

variable "kinesis_access_key_id" {
  sensitive = true
}

variable "kinesis_secret_key" {
  sensitive = true
}

# Object Storage variables
variable "s3_bucket_url" {
}

# Postgres CDC variables
variable "postgres_host" {
}

variable "postgres_port" {
  default = 5432
}

variable "postgres_database" {
  default = "postgres"
}

variable "postgres_username" {
  sensitive = true
  default   = "postgres"
}

variable "postgres_password" {
  sensitive = true
}

variable "postgres_schema" {
  default = "public"
}

variable "postgres_source_table" {
  default = "t1"
}

variable "postgres_target_table" {
  default = "public_t1"
}

data "clickhouse_api_key_id" "self" {
}

resource "clickhouse_service" "service" {
  name                 = var.service_name
  cloud_provider       = "aws"
  region               = var.region
  idle_scaling         = true
  idle_timeout_minutes = 5
  password_hash        = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0"
      description = "Anywhere"
    }
  ]

  endpoints = {
    mysql = {
      enabled = true
    }
  }

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

resource "clickhouse_clickpipe" "kafka_confluent" {

  name       = "E2E Test Kafka Confluent ClickPipe"
  service_id = clickhouse_service.service.id

  scaling = {
    replicas = 1
    replica_memory_gb = 0.5
    replica_cpu_millicores = 125
  }

  source = {
    kafka = {
      type           = "kafka"
      format         = "JSONEachRow"
      brokers        = var.kafka_brokers
      topics         = var.kafka_topics
      authentication = "PLAIN"
      credentials = {
        username = var.kafka_username
        password = var.kafka_password
      }
    }
  }

  destination = {
    table         = "e2e_test_table"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name = "radio"
        type = "String"
      },
      {
        name = "mcc"
        type = "String"
      },
      {
        name = "cell"
        type = "String"
      },
      {
        name = "lat"
        type = "String"
      },
      {
        name = "lon"
        type = "String"
      },
      {
        name = "created"
        type = "String"
      }
    ]
  }

  field_mappings = [
    {
      source_field      = "radio"
      destination_field = "radio"
    },
    {
      source_field      = "mcc"
      destination_field = "mcc"
    },
    {
      source_field      = "cell"
      destination_field = "cell"
    },
    {
      source_field      = "lat"
      destination_field = "lat"
    },
    {
      source_field      = "lon"
      destination_field = "lon"
    },
    {
      source_field      = "created"
      destination_field = "created"
    }
  ]
}

# Kinesis ClickPipe
resource "clickhouse_clickpipe" "kinesis" {
  name       = "E2E Test Kinesis ClickPipe"
  service_id = clickhouse_service.service.id

  scaling = {
    replicas = 1
    replica_memory_gb = 0.5
    replica_cpu_millicores = 125
  }

  source = {
    kinesis = {
      format        = "JSONEachRow"
      stream_name   = var.kinesis_stream_name
      region        = var.kinesis_region
      iterator_type = "TRIM_HORIZON"

      authentication = "IAM_USER"
      access_key = {
        access_key_id = var.kinesis_access_key_id
        secret_key    = var.kinesis_secret_key
      }
    }
  }

  destination = {
    table         = "e2e_kinesis_table"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name = "radio"
        type = "String"
      },
      {
        name = "mcc"
        type = "String"
      }
    ]
  }

  field_mappings = [
    {
      source_field      = "radio"
      destination_field = "radio"
    },
    {
      source_field      = "mcc"
      destination_field = "mcc"
    }
  ]
}

# Object Storage (S3) ClickPipe
resource "clickhouse_clickpipe" "object_storage" {
  name       = "E2E Test S3 ClickPipe"
  service_id = clickhouse_service.service.id

  source = {
    object_storage = {
      type   = "s3"
      format = "JSONEachRow"
      url    = var.s3_bucket_url
    }
  }

  destination = {
    table         = "e2e_s3_table"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

        columns = [
      {
        name = "count"
        type = "Int64"
      },
      {
        name = "category"
        type = "String"
      }
    ]
  }

  field_mappings = [
    {
      source_field      = "count"
      destination_field = "count"
    },
    {
      source_field      = "category"
      destination_field = "category"
    }
  ]
}

# CDC Infrastructure for Postgres
resource "clickhouse_clickpipe_cdc_infrastructure" "postgres_infra" {
  service_id             = clickhouse_service.service.id
  replica_cpu_millicores = 2000
  replica_memory_gb      = 8
}

# Postgres CDC ClickPipe
resource "clickhouse_clickpipe" "postgres_cdc" {
  name       = "E2E Test Postgres CDC ClickPipe"
  service_id = clickhouse_service.service.id

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
        replication_mode = "cdc"
		sync_interval_seconds              = 60
        pull_batch_size                    = 1000
        allow_nullable_columns             = true
        initial_load_parallelism           = 2
        snapshot_num_rows_per_partition    = 50000
        snapshot_number_of_parallel_tables = 2
        delete_on_merge                    = true
      }

      table_mappings = [
        {
          source_schema_name = var.postgres_schema
          source_table       = var.postgres_source_table
          target_table       = var.postgres_target_table
        }
      ]
    }
  }

  destination = {
    database = "default"
  }
}

output "service_id" {
  value = clickhouse_service.service.id
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "clickpipe_kafka_id" {
  value = clickhouse_clickpipe.kafka_confluent.id
}

output "clickpipe_kafka_state" {
  value = clickhouse_clickpipe.kafka_confluent.state
}

output "clickpipe_kinesis_id" {
  value = clickhouse_clickpipe.kinesis.id
}

output "clickpipe_kinesis_state" {
  value = clickhouse_clickpipe.kinesis.state
}

output "clickpipe_s3_id" {
  value = clickhouse_clickpipe.object_storage.id
}

output "clickpipe_s3_state" {
  value = clickhouse_clickpipe.object_storage.state
}

output "clickpipe_postgres_id" {
  value = clickhouse_clickpipe.postgres_cdc.id
}

output "clickpipe_postgres_state" {
  value = clickhouse_clickpipe.postgres_cdc.state
}
