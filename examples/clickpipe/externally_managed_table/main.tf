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

variable "table_name" {
  description = "Table name"
  type        = string
}

variable "table_columns" {
  description = "Table columns"
  type = list(object({
    name = string
    type = string
  }))
}

variable "field_mappings" {
  description = "Field mappings"
  type = list(object({
    source_field      = string
    destination_field = string
  }))
}

resource "clickhouse_clickpipe" "kafka_confluent" {
  name        = "Confluent 🚀 ClickPipe"
  description = "Data pipeline from Confluent to ClickHouse"

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
    }
  }

  destination = {
    table         = var.table_name
    managed_table = false

    columns = var.table_columns
  }

  field_mappings = var.field_mappings
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.kafka_confluent.id
}