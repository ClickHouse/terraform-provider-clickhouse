variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse service ID"
}

variable "msk_cluster_arn" {
  description = "MSK cluter ARN"
}

variable "msk_authentication" {
  description = "MSK authentication"
  default     = "SASL_SCRAM"
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "endpoint" {
  service_id         = var.service_id
  description        = "Reverse private endpoint for ClickPipes"
  type               = "MSK_MULTI_VPC"
  msk_cluster_arn    = var.msk_cluster_arn
  msk_authentication = var.msk_authentication
}

output "id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.id
}

output "msk_vpc_connection_id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.endpoint_id
}

output "dns_names" {
  value = concat(
    clickhouse_clickpipes_reverse_private_endpoint.endpoint.dns_names != null ? clickhouse_clickpipes_reverse_private_endpoint.endpoint.dns_names : [],
    clickhouse_clickpipes_reverse_private_endpoint.endpoint.private_dns_names != null ? clickhouse_clickpipes_reverse_private_endpoint.endpoint.private_dns_names : []
  )
}

output "status" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.status
}
