variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
}

variable "kafka_brokers" {
}

variable "kafka_topics" {
}

variable "kafka_username" {
  sensitive   = true
}

variable "kafka_password" {
  sensitive   = true
}

resource "clickhouse_clickpipe" "kafka_redpanda" {
  name        = "Redpanda 🚀 ClickPipe"

  service_id = var.service_id

  scaling = {
    replicas = 1
  }



  source = {
    kafka = {
      type    = "redpanda"
      format  = "JSONEachRow"
      brokers = var.kafka_brokers
      topics  = var.kafka_topics

      authentication = "SCRAM-SHA-512"
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
  value = clickhouse_clickpipe.kafka_redpanda.id
}