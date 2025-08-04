variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse service ID"
}

variable "azure_connection_string" {
  description = "Azure Blob Storage connection string"
  sensitive   = true
}

variable "azure_container_name" {
  description = "Azure Blob Storage container name"
}

variable "azure_path" {
  description = "Path to the file(s) within the Azure container"
  default     = "data/*.json"
}

resource "clickhouse_clickpipe" "azure_blob" {
  name        = "Azure Blob Storage 🚀 ClickPipe"
  service_id = var.service_id
  state = "Running"
  source = {
    object_storage = {
      type    = "azureblobstorage"
      format  = "JSONEachRow"

      path                  = var.azure_path
      azure_container_name  = var.azure_container_name
      connection_string     = var.azure_connection_string

      authentication = "CONNECTION_STRING"
    }
  }

  destination = {
    table         = "my_azure_data_table"
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
      }, {
        name = "name"
        type = "String"
      }, {
        name = "timestamp"
        type = "DateTime"
      }
    ]
  }

  field_mappings = [
    {
      source_field      = "user_id"
      destination_field = "id"
    }, {
      source_field      = "user_name"
      destination_field = "name"
    }, {
      source_field      = "created_at"
      destination_field = "timestamp"
    }
  ]
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.azure_blob.id
}
