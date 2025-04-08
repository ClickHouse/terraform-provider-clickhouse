locals {
  rpe_msk_authentication_mapping = {
    "SCRAM-SHA-512" = "SASL_SCRAM"
    "IAM_ROLE"      = "SASL_IAM"
    "IAM_USER"     = "SASL_IAM"
  }
  
  // In this example we map from a ClickPipe authentication method
  // into a MSK multi-VPC authentication method.
  rpe_msk_authentication = local.rpe_msk_authentication_mapping[var.msk_authentication]
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "endpoint" {
  service_id                = var.service_id
  description               = "Reverse private endpoint for my ClickPipe"
  type                      = "MSK_MULTI_VPC"
  msk_cluster_arn = var.msk_cluster_arn
  msk_authentication = local.rpe_msk_authentication
}

output "reverse_private_endpoint_id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.id
}

output "msk_vpc_connection_id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.endpoint.endpoint_id
}
