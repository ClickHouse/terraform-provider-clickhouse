variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse service ID"
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

variable "schema_registry_url" {
  description = "Schema Registry URL"
}

variable "schema_registry_username" {
  description = "Schema Registry Username"
  sensitive   = true
}

variable "schema_registry_password" {
  description = "Schema Registry Password"
  sensitive   = true
}

resource "clickhouse_clickpipe" "kafka_schema_registry" {
  name        = "Schema Registry 🚀 ClickPipe"
  description = "Data pipeline with use of Schema Registry"

  service_id = var.service_id

  scaling = {
    replicas = 1
  }

  state = "Running"

  source = {
    kafka = {
      type    = "confluent"
      format  = "AvroConfluent"
      brokers = var.kafka_brokers
      topics  = var.kafka_topics

      credentials = {
        username = var.kafka_username
        password = var.kafka_password
      }

      schema_registry = {
        url            = var.schema_registry_url
        authentication = "PLAIN"
        credentials = {
          username = var.schema_registry_username
          password = var.schema_registry_password
        }
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
  value = clickhouse_clickpipe.kafka_schema_registry.id
}
