variable "organization_id" {
  type = string
}

variable "token_key" {
  type      = string
  sensitive = true
}

variable "token_secret" {
  type      = string
  sensitive = true
}

variable "service_name" {
  type    = string
  default = "My Terraform Service"
}

variable "region" {
  type    = string
  default = "us-east-2"
}

variable "release_channel" {
  type    = string
  default = "default"
  validation {
    condition     = contains(["default", "fast", "slow"], var.release_channel)
    error_message = "Release channel can be 'default', 'fast' or 'slow'."
  }
}

variable "suffix" {
  type    = string
  default = ""
}

locals {
  # Function names allow only letters, numbers and underscores, so the e2e
  # suffix has to be sanitized before it can make the name unique.
  udf_function_name = "tf_test_udf${replace(var.suffix, "/[^A-Za-z0-9_]/", "_")}"
}

resource "clickhouse_service" "service" {
  name                 = var.service_name
  cloud_provider       = "aws"
  region               = var.region
  release_channel      = var.release_channel
  idle_scaling         = true
  idle_timeout_minutes = 5
  password_hash        = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "0.0.0.0"
      description = "Anywhere"
    }
  ]

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120

  backup_configuration = {
    backup_period_in_hours           = 24
    backup_retention_period_in_hours = 24
    backup_start_time                = null
  }
}

# Zip the UDF source with the archive provider so the source stays reviewable
# in the repo instead of a committed binary blob. The API requires the
# executable entry point to be named main.py at the root of the archive.
data "archive_file" "udf" {
  type        = "zip"
  source_file = "${path.module}/src/main.py"
  output_path = "${path.module}/build/udf_source.zip"
}

resource "clickhouse_udf" "echo_string" {
  function_name = local.udf_function_name
  runtime       = "python3.11"
  type          = "executable"
  return_type   = "String"

  arguments = [
    { name = "input", type = "String" },
  ]

  source_archive_path = data.archive_file.udf.output_path
  source_archive_hash = data.archive_file.udf.output_base64sha256
}

resource "clickhouse_udf_attachment" "echo_string" {
  function_name = clickhouse_udf.echo_string.function_name
  service_id    = clickhouse_service.service.id
  version       = clickhouse_udf.echo_string.version
}

output "service_endpoints" {
  value = clickhouse_service.service.endpoints
}

output "service_iam" {
  value = clickhouse_service.service.iam_role
}

output "udf_version" {
  value = clickhouse_udf.echo_string.version
}

output "udf_attachment_status" {
  value = clickhouse_udf_attachment.echo_string.status
}
