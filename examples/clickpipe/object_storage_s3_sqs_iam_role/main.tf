variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse Cloud service ID"
}

variable "bucket_url" {
  description = "S3 bucket URL pattern (e.g., s3://my-bucket/path/*.json)"
}

variable "sqs_queue_url" {
  description = "SQS queue URL for S3 event notifications (e.g., https://sqs.us-east-1.amazonaws.com/123456789012/my-queue)"
}

variable "iam_role" {
  description = "ARN of the IAM role with permissions to read from S3 and receive SQS messages"
  sensitive   = true
}

# S3 ClickPipe with continuous ingestion using SQS event notifications
# This example demonstrates event-based continuous ingestion where new files
# are detected via S3 event notifications sent to an SQS queue, rather than
# polling S3 for new files in lexicographical order.
resource "clickhouse_clickpipe" "s3_sqs_continuous" {
  name       = "S3 Continuous ClickPipe with SQS (IAM Role)"
  service_id = var.service_id

  source = {
    object_storage = {
      type   = "s3"
      format = "JSONEachRow"
      url    = var.bucket_url

      # Enable continuous ingestion with event-based processing
      is_continuous = true
      queue_url     = var.sqs_queue_url

      # IAM role authentication - recommended for AWS services
      authentication = "IAM_ROLE"
      iam_role       = var.iam_role
    }
  }

  destination = {
    table         = "s3_events_data"
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
  value       = clickhouse_clickpipe.s3_sqs_continuous.id
  description = "The ID of the created ClickPipe"
}

output "clickpipe_state" {
  value       = clickhouse_clickpipe.s3_sqs_continuous.state
  description = "The current state of the ClickPipe"
}
