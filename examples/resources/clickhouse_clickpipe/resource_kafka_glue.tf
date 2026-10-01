# Requires the AWS Glue schema registry to be enabled for the organization.
resource "clickhouse_clickpipe" "kafka_glue" {
  name       = "My Kafka Glue ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    kafka = {
      type = "msk"
      # Use "Protobuf" to decode Protobuf records through the same registry.
      format  = "AvroConfluent"
      brokers = "b-1.my-cluster.kafka.us-east-1.amazonaws.com:9098"
      topics  = "orders"

      authentication = "IAM_ROLE"
      iam_role       = "arn:aws:iam::123456789012:role/clickpipes"

      schema_registry = {
        type               = "glue"
        glue_region        = "us-east-1"
        glue_registry_name = "orders-registry"
        # Optional with IAM broker authentication, where it defaults to the broker's IAM identity.
        # Required for any other broker authentication.
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
