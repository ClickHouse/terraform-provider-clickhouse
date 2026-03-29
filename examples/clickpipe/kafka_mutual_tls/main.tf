variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
}

variable "kafka_brokers" {
}

variable "kafka_topics" {
}

variable "client_certificate" {
  sensitive = true
}

variable "client_private_key" {
  sensitive = true
}

variable "ca_certificate" {
}

resource "clickhouse_clickpipe" "kafka_mutual_tls" {
  name = "Kafka mTLS ClickPipe"

  service_id = var.service_id

  scaling = {
    replicas = 1
  }

  source = {
    kafka = {
      type    = "kafka"
      format  = "JSONEachRow"
      brokers = var.kafka_brokers
      topics  = var.kafka_topics

      authentication = "MUTUAL_TLS"

      ca_certificate = var.ca_certificate

      credentials = {
        certificate = var.client_certificate
        private_key = var.client_private_key
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
  value = clickhouse_clickpipe.kafka_mutual_tls.id
}
