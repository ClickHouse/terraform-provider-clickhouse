variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
}

variable "kafka_brokers" {
}

variable "kafka_topics" {
}

variable "iam_access_key_id" {
  sensitive   = true
}

variable "iam_secret_key" {
  sensitive   = true
}

resource "clickhouse_clickpipe" "kafka_msk" {
  name        = "MSK 🚀 ClickPipe"

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

      authentication = "IAM_USER"
      credentials = {
        access_key_id = var.iam_access_key_id
        secret_key = var.iam_secret_key
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
  value = clickhouse_clickpipe.kafka_msk.id
}