variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
}

variable "vpc_resource_configuration_id" {
}

variable "vpc_resource_share_arn" {
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "endpoint" {
  service_id                = var.service_id
  type                      = "VPC_RESOURCE"
  vpc_resource_configuration_id = var.vpc_resource_configuration_id
  vpc_resource_share_arn    = var.vpc_resource_share_arn
}

output "id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.id
}

output "endpoint_id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.endpoint_id
}

output "dns_names" {
  value = concat(
    clickhouse_clickpipes_reverse_private_endpoint.endpoint.dns_names,
    clickhouse_clickpipes_reverse_private_endpoint.endpoint.private_dns_names != null ? clickhouse_clickpipes_reverse_private_endpoint.endpoint.private_dns_names : []
  )
}

output "status" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.status
}
