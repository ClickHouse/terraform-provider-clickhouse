resource "random_id" "clickpipes_suffix" {
  byte_length = 4
}

locals {
  snapshot_staging_path = "gs://${google_storage_bucket.clickpipes_staging_bucket.name}/${random_id.clickpipes_suffix.hex}/"
}

resource "clickhouse_clickpipe" "bigquery_snapshot" {
  name = "BigQuery Snapshot ClickPipe"

  service_id = var.service_id

  source = {
    bigquery = {
      snapshot_staging_path = local.snapshot_staging_path

      credentials = {
        service_account_file = google_service_account_key.clickpipes_key.private_key
      }

      settings = {
        replication_mode = "snapshot"
      }

      table_mappings = [for table_name in var.bigquery_table_names : {
        source_dataset_name = var.bigquery_dataset_id
        source_table        = table_name
        target_table        = "${table_name}_${random_id.clickpipes_suffix.hex}"
      }]
    }
  }

  destination = {
    database = "default"
  }
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.bigquery_snapshot.id
}

output "clickpipe_state" {
  value = clickhouse_clickpipe.bigquery_snapshot.state
}
