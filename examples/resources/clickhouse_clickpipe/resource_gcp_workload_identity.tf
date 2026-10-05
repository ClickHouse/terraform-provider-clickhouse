data "clickhouse_clickpipes_service_context" "service" {
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"
}

resource "google_project_iam_member" "gcmk_client" {
  project = "customer-source-project"
  role    = "roles/managedkafka.client"
  member  = "serviceAccount:${data.clickhouse_clickpipes_service_context.service.gcp_workload_identity.principal}"
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

resource "google_project_iam_member" "bigquery_data_viewer" {
  project = "customer-source-project"
  role    = "roles/bigquery.dataViewer"
  member  = "serviceAccount:${data.clickhouse_clickpipes_service_context.service.gcp_workload_identity.principal}"
}

resource "google_project_iam_member" "bigquery_job_user" {
  project = "customer-source-project"
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${data.clickhouse_clickpipes_service_context.service.gcp_workload_identity.principal}"
}

resource "google_project_iam_member" "bigquery_staging_object_admin" {
  project = "customer-source-project"
  role    = "roles/storage.objectAdmin"
  member  = "serviceAccount:${data.clickhouse_clickpipes_service_context.service.gcp_workload_identity.principal}"
}

resource "google_project_iam_member" "bigquery_staging_bucket_viewer" {
  project = "customer-source-project"
  role    = "roles/storage.bucketViewer"
  member  = "serviceAccount:${data.clickhouse_clickpipes_service_context.service.gcp_workload_identity.principal}"
}

resource "clickhouse_clickpipe" "bigquery_workload_identity" {
  name       = "BigQuery workload identity ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    bigquery = {
      authentication        = "SERVICE_ACCOUNT_WORKLOAD_IDENTITY"
      project_id            = "customer-source-project"
      snapshot_staging_path = "gs://customer-staging-bucket/clickpipes/"

      settings = {
        replication_mode = "snapshot"
      }

      table_mappings = [{
        source_dataset_name = "analytics"
        source_table        = "events"
        target_table        = "events"
      }]
    }
  }

  destination = {
    database = "default"
  }

  depends_on = [
    google_project_iam_member.bigquery_data_viewer,
    google_project_iam_member.bigquery_job_user,
    google_project_iam_member.bigquery_staging_object_admin,
    google_project_iam_member.bigquery_staging_bucket_viewer,
  ]
}
