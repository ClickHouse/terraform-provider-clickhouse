variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_name" {
  type = string
  default = "My Terraform Service"
}

variable "region" {
  type = string
  default = "us-east-2"
}

variable "kafka_brokers" {
  description = "Kafka brokers"
}

variable "kafka_topics" {
  description = "Kafka topics"
}

variable "kafka_username" {
  description = "Username"
  sensitive   = true
}

variable "kafka_password" {
  description = "Password"
  sensitive   = true
}

resource "clickhouse_service" "service" {
  name                      = var.service_name
  cloud_provider            = "aws"
  region                    = var.region
  tier                      = "development"
  idle_scaling              = true
  idle_timeout_minutes      = 5
  password_hash             = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0"
      description = "Anywhere"
    }
  ]
}

resource "clickhouse_clickpipe" "kafka_confluent" {
  name        = "🚀 ClickPipe created with Terraform"
  description = "Data pipeline from Confluent to ClickHouse"

  service_id = clickhouse_service.service.id

  scaling = {
    replicas = 1
  }

  state = "Running"

  source = {
    kafka = {
      type    = "confluent"
      format  = "JSONEachRow"
      brokers = var.kafka_brokers
      topics  = var.kafka_topics

      credentials = {
        username = var.kafka_username
        password = var.kafka_password
      }
    }
  }

  destination = {
    table         = "my_table"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name = "my_field1"
        type = "String"
      }, {
        name = "my_field2"
        type = "UInt64"
      }
    ]
  }

  field_mappings = [
    {
      source_field      = "my_field"
      destination_field = "my_field1"
    }
  ]
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.kafka_confluent.id
}