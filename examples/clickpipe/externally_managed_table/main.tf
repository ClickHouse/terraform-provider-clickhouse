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

variable "table_name" {
  type        = string
}

variable "table_columns" {
  type = list(object({
    name = string
    type = string
  }))
}

variable "field_mappings" {
  type = list(object({
    source_field      = string
    destination_field = string
  }))
}

resource "clickhouse_clickpipe" "kafka_confluent" {
  name        = "Confluent 🚀 ClickPipe"

  service_id = var.service_id

  scaling = {
    replicas = 1
  }



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