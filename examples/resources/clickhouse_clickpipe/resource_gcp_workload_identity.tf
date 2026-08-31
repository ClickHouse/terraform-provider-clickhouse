data "clickhouse_clickpipes_service_context" "gcmk" {
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"
}

resource "google_project_iam_member" "gcmk_client" {
  project = "customer-source-project"
  role    = "roles/managedkafka.client"
  member  = "serviceAccount:${data.clickhouse_clickpipes_service_context.gcmk.gcp_workload_identity.principal}"
}

resource "clickhouse_clickpipe" "gcmk_workload_identity" {
  name       = "GCMK workload identity ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    kafka = {
      type           = "gcmk"
      format         = "JSONEachRow"
      brokers        = "bootstrap.kafka.us-central1.managedkafka.customer-source-project.cloud.goog:9092"
      topics         = "events"
      authentication = "SERVICE_ACCOUNT_WORKLOAD_IDENTITY"
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

  field_mappings = [
    {
      source_field      = "id"
      destination_field = "id"
    }
  ]

  depends_on = [google_project_iam_member.gcmk_client]
}
