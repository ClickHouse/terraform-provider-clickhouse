# Requires Kinesis Protobuf schema upload to be enabled for the organization.
resource "clickhouse_clickpipe" "kinesis_protobuf" {
  name       = "My Kinesis Protobuf ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    kinesis = {
      format          = "Protobuf"
      stream_name     = "events"
      region          = "us-east-1"
      iterator_type   = "TRIM_HORIZON"
      authentication  = "IAM_ROLE"
      iam_role        = "arn:aws:iam::123456789012:role/clickpipes"
      protobuf_schema = filebase64("${path.module}/event.proto")
    }
  }

  destination = {
    table         = "events"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name = "id"
        type = "String"
      }
    ]
  }
}
