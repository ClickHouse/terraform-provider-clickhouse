variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse service ID"
}

variable "bucket_url" {
  description = "S3-compatible bucket URL"
}

variable "iam_access_key_id" {
  description = "IAM access key ID"
  sensitive   = true
}

variable "iam_secret_key" {
  description = "IAM secret key"
  sensitive   = true
}

resource "clickhouse_clickpipe" "kafka_s3" {
  name        = "S3 🚀 ClickPipe with IAM user"
  description = "Data pipeline from S3 to ClickHouse"

  service_id = var.service_id
  
  state = "Running"
  
  source = {
    object_storage = {
      type    = "s3"
      format  = "JSONEachRow"

      url = var.bucket_url

      authentication = "IAM_USER"
      access_key = {
        access_key_id = var.iam_access_key_id
        secret_key = var.iam_secret_key
      }
    }
  }

  destination = {
    table         = "my_data_table"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name = "my_field1"
        type = "String"
      }, {
        name = "my_field2"
        type = "UInt64"
      }
    ]
  }

  field_mappings = [
    {
      source_field      = "my_field"
      destination_field = "my_field1"
    }
  ]
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.kafka_s3.id
}