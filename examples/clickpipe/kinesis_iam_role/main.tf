resource "clickhouse_clickpipe" "kinesis_iam_role" {
  service_id  = var.service_id
  name        = var.pipe_name
  description = var.pipe_description

  source = {
    kinesis = {
      format        = "JSONEachRow"
      stream_name   = var.kinesis_stream_name
      region        = var.aws_region
      iterator_type = "LATEST"

      # Set to true to use enhanced fan-out consumer (optional)
      use_enhanced_fan_out = true

      # Using IAM role authentication
      authentication = "IAM_ROLE"
      iam_role       = var.iam_role_arn
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
