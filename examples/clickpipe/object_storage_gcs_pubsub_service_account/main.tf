variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse Cloud service ID"
}

variable "bucket_url" {
  description = "GCS bucket URL pattern (e.g., gs://my-bucket/path/*.json)"
}

variable "pubsub_subscription" {
  description = "Pub/Sub subscription for GCS event notifications (e.g., projects/my-project/subscriptions/my-subscription)"
}

variable "service_account_key" {
  description = "Base64-encoded GCP service account JSON key"
  sensitive   = true
}

# GCS ClickPipe with continuous ingestion using Pub/Sub event notifications
# This example demonstrates event-based continuous ingestion where new files
# are detected via GCS event notifications sent to a Pub/Sub subscription,
# rather than polling GCS for new files in lexicographical order.
resource "clickhouse_clickpipe" "gcs_pubsub_continuous" {
  name       = "GCS Continuous ClickPipe with Pub/Sub (Service Account)"
  service_id = var.service_id

  source = {
    object_storage = {
      type   = "gcs"
      format = "JSONEachRow"
      url    = var.bucket_url

      # Enable continuous ingestion with event-based processing
      is_continuous = true
      queue_url     = var.pubsub_subscription

      # Service account authentication for GCS
      authentication    = "SERVICE_ACCOUNT"
      service_account_key = var.service_account_key
    }
  }

  destination = {
    table         = "gcs_events_data"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }

      sorting_key = ["timestamp"]
    }

    columns = [
      {
        name = "id"
        type = "String"
      },
      {
        name = "timestamp"
        type = "DateTime64(3)"
      },
      {
        name = "event_type"
        type = "String"
      },
      {
        name = "data"
        type = "String"
      }
    ]
  }

  field_mappings = [
    {
      source_field      = "id"
      destination_field = "id"
    },
    {
      source_field      = "timestamp"
      destination_field = "timestamp"
    },
    {
      source_field      = "event_type"
      destination_field = "event_type"
    },
    {
      source_field      = "data"
      destination_field = "data"
    }
  ]
}

output "clickpipe_id" {
  value       = clickhouse_clickpipe.gcs_pubsub_continuous.id
  description = "The ID of the created ClickPipe"
}

output "clickpipe_state" {
  value       = clickhouse_clickpipe.gcs_pubsub_continuous.state
  description = "The current state of the ClickPipe"
}
