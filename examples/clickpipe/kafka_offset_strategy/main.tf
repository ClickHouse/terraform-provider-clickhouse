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

resource "clickhouse_clickpipe" "kafka_offset_strategy" {
  name        = "Offset strategy 🚀 ClickPipe"
  description = "Data pipeline with a custom offset strategy"

  service_id = var.service_id

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
      
      offset = {
        strategy = "from_timestamp"
        timestamp = "2021-01-01T00:00"
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
  value = clickhouse_clickpipe.kafka_offset_strategy.id
}