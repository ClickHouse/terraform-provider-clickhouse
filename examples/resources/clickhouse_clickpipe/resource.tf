resource "clickhouse_clickpipe" "kafka_clickpipe" {
  name           = "My Kafka ClickPipe"
  description    = "Data pipeline from Kafka to ClickHouse"

  service_id     = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  scaling {
    replicas = 1
  }

  state = "Running"

  source {
    kafka {
      type = "confluent"
      format = "JSONEachRow"
      brokers = "my-kafka-broker:9092"
      topics = "my_topic"

      consumer_group = "clickpipe-test"

      credentials {
        username = "user"
        password = "***"
      }
    }
  }

  destination {
    table    = "my_table"
    managed_table = true
    
    tableDefinition {
      engine {
        type = "MergeTree"
      }
    }

    columns {
      name = "my_field1"
      type = "String"
    }

    columns {
      name = "my_field2"
      type = "UInt64"
    }
  }

  field_mappings = [
    {
      source_field = "my_field"
      destination_field = "my_field1"
    }
  ]
}
