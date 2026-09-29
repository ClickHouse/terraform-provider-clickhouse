# Requires the AWS Glue schema registry for Kinesis to be enabled for the organization.
resource "clickhouse_clickpipe" "kinesis_glue" {
  name       = "My Kinesis Glue ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    kinesis = {
      # Use "Protobuf" to decode Protobuf records through the same registry.
      format         = "AvroConfluent"
      stream_name    = "orders"
      region         = "us-east-1"
      iterator_type  = "TRIM_HORIZON"
      authentication = "IAM_ROLE"
      iam_role       = "arn:aws:iam::123456789012:role/clickpipes"

      schema_registry = {
        type               = "glue"
        glue_region        = "us-east-1"
        glue_registry_name = "orders-registry"
        # Optional. Defaults to the IAM identity of the Kinesis source.
        glue_role_arn = "arn:aws:iam::123456789012:role/GlueRegistryAccess"
      }
    }
  }

  destination = {
    table         = "orders"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name = "order_id"
        type = "String"
      }
    ]
  }
}
