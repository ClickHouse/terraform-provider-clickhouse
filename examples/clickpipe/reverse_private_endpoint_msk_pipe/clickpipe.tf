locals {
  // MSK brokers for multi-VPC connectivity ports starting from 14001 incremented by a broker number
  msk_brokers = [for i, dns in clickhouse_clickpipes_reverse_private_endpoint.endpoint.private_dns_names : "${dns}:${i + 14001}"]
}

resource "clickhouse_clickpipe" "msk" {
  name        = "MSK pipe using Reverse Private Endpoint"
  description = "This pipe is using a secure private endpoint to connect to MSK"

  service_id = var.service_id

  scaling = {
    replicas = 1
  }

  state = "Running"

  source = {
    kafka = {
      type    = "msk"
      format  = "JSONEachRow"
      brokers = join(",", local.msk_brokers)
      topics  = var.kafka_topic

      authentication = var.msk_authentication
      credentials = {
        username = var.msk_scram_user
        password = var.msk_scram_password
      }

      reverse_private_endpoint_ids = [
        clickhouse_clickpipes_reverse_private_endpoint.endpoint.id
      ]
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
  value = clickhouse_clickpipe.msk.id
}