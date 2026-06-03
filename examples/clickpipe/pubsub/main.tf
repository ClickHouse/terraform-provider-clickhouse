resource "clickhouse_clickpipe" "pubsub" {
  service_id = var.service_id
  name       = var.pipe_name

  source = {
    pubsub = {
      format         = "JSONEachRow"
      project_id     = var.gcp_project_id
      topic          = var.pubsub_topic
      authentication = "SERVICE_ACCOUNT"
      seek_type      = "latest"

      service_account_key = {
        service_account_file = var.gcp_service_account_b64
      }
    }
  }

  destination = {
    table         = var.table
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
      },
      {
        name = "my_field2"
        type = "UInt64"
      },
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
  value = clickhouse_clickpipe.pubsub.id
}
