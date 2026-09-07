resource "clickhouse_clickpipe" "kafka_protobuf" {
  name       = "My Kafka Protobuf ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    kafka = {
      type            = "confluent"
      format          = "Protobuf"
      brokers         = "my-kafka-broker:9092"
      topics          = "events"
      protobuf_schema = filebase64("${path.module}/event.proto")

      credentials = {
        username = "user"
        password = "***"
      }
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
