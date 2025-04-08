resource "clickhouse_clickpipes_reverse_private_endpoint" "vpc_endpoint_service" {
  service_id                = "3a10a385-ced2-452e-abb8-908c80976a8f"
  description               = "VPC_ENDPOINT_SERVICE reverse private endpoint for ClickPipes"
  type                      = "VPC_ENDPOINT_SERVICE"
  vpc_endpoint_service_name = "com.amazonaws.vpce.eu-west-1.vpce-svc-080826a65b5b27d4e"
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "vpc_resource" {
  service_id                    = "3a10a385-ced2-452e-abb8-908c80976a8f"
  description                   = "VPC_RESOURCE reverse private endpoint for ClickPipes"
  type                          = "VPC_RESOURCE"
  vpc_resource_configuration_id = "rcfg-1a2b3c4d5e6f7g8h9"
  vpc_resource_share_arn        = "arn:aws:ram:us-east-1:123456789012:resource-share/1a2b3c4d-5e6f-7g8h-9i0j-k1l2m3n4o5p6"
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "msk_multi_vpc" {
  service_id         = "3a10a385-ced2-452e-abb8-908c80976a8f"
  description        = "MSK_MULTI_VPC reverse private endpoint for ClickPipes"
  type               = "MSK_MULTI_VPC"
  msk_cluster_arn    = "arn:aws:kafka:us-east-1:123456789012:cluster/ClickHouse-Cluster/1a2b3c4d-5e6f-7g8h-9i0j-k1l2m3n4o5p6-1"
  msk_authentication = "SASL_IAM"
}
