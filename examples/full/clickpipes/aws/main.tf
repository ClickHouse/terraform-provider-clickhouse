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

output "service_id" {
  value = clickhouse_service.service.id
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.kafka_confluent.id
}

output "clickpipe_state" {
  value = clickhouse_clickpipe.kafka_confluent.state
}
