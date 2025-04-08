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

variable "iam_role" {
  description = "IAM role ARN"
}

resource "clickhouse_clickpipe" "kafka_msk" {
  name        = "MSK 🚀 ClickPipe"
  description = "Data pipeline from MSK to ClickHouse"

  service_id = var.service_id

  scaling = {
    replicas = 1
  }

  state = "Running"

  source = {
    kafka = {
      type    = "msk"
      format  = "JSONEachRow"
      brokers = var.kafka_brokers
      topics  = var.kafka_topics

      authentication = "IAM_ROLE"
      iam_role       = var.iam_role
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
  value = clickhouse_clickpipe.kafka_msk.id
}