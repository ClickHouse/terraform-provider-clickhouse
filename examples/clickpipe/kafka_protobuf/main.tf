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

variable "schema_registry_url" {
}

variable "schema_registry_username" {
  sensitive   = true
}

variable "schema_registry_password" {
  sensitive   = true
}

resource "clickhouse_clickpipe" "kafka_protobuf" {
  name        = "Terraform Kafka Protobuf ClickPipe"

  service_id = var.service_id

  scaling = {
    replicas = 1
  }

  source = {
    kafka = {
      type    = "confluent"
      format  = "Protobuf"
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
    table         = "tf_basic_types_proto"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name = "id"
        type = "Int32"
      }
    ]
  }
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.kafka_protobuf.id
}

output "clickpipe_state" {
  value = clickhouse_clickpipe.kafka_protobuf.state
}
