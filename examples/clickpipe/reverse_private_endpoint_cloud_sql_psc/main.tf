variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse Cloud service ID"
}

variable "cloud_sql_service_attachment" {
  description = "Cloud SQL Private Service Connect service attachment URI. Format: projects/{project}/regions/{region}/serviceAttachments/{name}."
}

variable "cloud_sql_private_dns_name" {
  description = "Custom private DNS name ClickPipes should use as the Cloud SQL source host."
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "cloud_sql" {
  service_id             = var.service_id
  description            = "Cloud SQL PSC reverse private endpoint"
  type                   = "GCP_PSC_SERVICE_ATTACHMENT"
  gcp_service_attachment = var.cloud_sql_service_attachment

  custom_private_dns_mappings = [
    {
      private_dns_name = var.cloud_sql_private_dns_name
    }
  ]
}

output "reverse_private_endpoint_id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.cloud_sql.id
}

output "endpoint_id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.cloud_sql.endpoint_id
}

output "cloud_sql_host" {
  description = "Use this value as the Postgres or MySQL ClickPipe source host."
  value       = var.cloud_sql_private_dns_name
}

output "dns_names" {
  value = clickhouse_clickpipes_reverse_private_endpoint.cloud_sql.dns_names
}

output "private_dns_names" {
  value = clickhouse_clickpipes_reverse_private_endpoint.cloud_sql.private_dns_names
}

output "status" {
  value = clickhouse_clickpipes_reverse_private_endpoint.cloud_sql.status
}
