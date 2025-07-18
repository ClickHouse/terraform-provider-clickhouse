resource "clickhouse_clickpipe" "kinesis_iam_user" {
  service_id  = var.service_id
  name        = var.pipe_name

  source = {
    kinesis = {
      format        = "JSONEachRow"
      stream_name   = var.kinesis_stream_name
      region        = var.aws_region
      iterator_type = "TRIM_HORIZON"

      authentication = "IAM_USER"
      access_key = {
        access_key_id = var.aws_access_key
        secret_key    = var.aws_secret_key
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
  value = clickhouse_clickpipe.kinesis_iam_user.id
}
