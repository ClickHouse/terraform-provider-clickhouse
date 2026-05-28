resource "clickhouse_clickpipe" "pubsub_latest" {
  name       = "pubsub-latest"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  scaling = {
    replicas               = 2
    replica_cpu_millicores = 250
    replica_memory_gb      = 1.0
  }

  source = {
    pubsub = {
      format         = "JSONEachRow"
      project_id     = "my-gcp-project"
      topic          = "events"
      authentication = "SERVICE_ACCOUNT"
      seek_type      = "latest"

      service_account_key = {
        service_account_file = var.gcp_service_account_b64
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
        type = "UInt64"
      }
    ]
  }
}

resource "clickhouse_clickpipe" "pubsub_timestamp" {
  name       = "pubsub-from-timestamp"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    pubsub = {
      format          = "Avro"
      project_id      = "my-gcp-project"
      topic           = "events"
      authentication  = "SERVICE_ACCOUNT"
      seek_type       = "timestamp"
      seek_timestamp  = "2026-04-10T12:00:00Z"
      filter          = "attributes.env = \"prod\""
      enable_ordering = false
      ack_deadline    = 120

      service_account_key = {
        service_account_file = var.gcp_service_account_b64
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
        type = "UInt64"
      }
    ]
  }
}
