resource "clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns" "example" {
  service_id                  = "3a10a385-ced2-452e-abb8-908c80976a8f"
  reverse_private_endpoint_id = "12345678-1234-1234-1234-123456789012"

  mapping = [
    {
      private_dns_name = "my-service.example.com"
    }
  ]
}

# MongoDB GROUP with one CHILD per node. Supply an existing resource gateway,
# the ClickHouse service ID, and the ClickHouse AWS account used for RAM sharing.
variable "resource_gateway_id" {
  type = string
}

variable "service_id" {
  type = string
}

variable "clickhouse_aws_account_id" {
  type = string
}

resource "aws_vpclattice_resource_configuration" "mongo" {
  name                        = "mongo"
  type                        = "GROUP"
  resource_gateway_identifier = var.resource_gateway_id
  protocol                    = "TCP"
  port_ranges                 = ["27017"]
}

resource "aws_vpclattice_resource_configuration" "node_00" {
  name                            = "mongo-node-00"
  type                            = "CHILD"
  resource_configuration_group_id = aws_vpclattice_resource_configuration.mongo.id

  resource_configuration_definition {
    dns_resource {
      domain_name     = "node-00.mongo.example.com"
      ip_address_type = "IPV4"
    }
  }
}

resource "aws_vpclattice_resource_configuration" "node_01" {
  name                            = "mongo-node-01"
  type                            = "CHILD"
  resource_configuration_group_id = aws_vpclattice_resource_configuration.mongo.id

  resource_configuration_definition {
    dns_resource {
      domain_name     = "node-01.mongo.example.com"
      ip_address_type = "IPV4"
    }
  }
}

resource "aws_ram_resource_share" "mongo" {
  name                      = "mongo-clickpipes"
  allow_external_principals = true
}

resource "aws_ram_resource_association" "mongo" {
  resource_arn       = aws_vpclattice_resource_configuration.mongo.arn
  resource_share_arn = aws_ram_resource_share.mongo.arn
}

resource "aws_ram_principal_association" "clickhouse" {
  principal          = var.clickhouse_aws_account_id
  resource_share_arn = aws_ram_resource_share.mongo.arn
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "mongo" {
  service_id                    = var.service_id
  description                   = "MongoDB GROUP reverse private endpoint"
  type                          = "VPC_RESOURCE"
  vpc_resource_configuration_id = aws_vpclattice_resource_configuration.mongo.id
  vpc_resource_share_arn        = aws_ram_resource_share.mongo.arn
  wait_for_ready                = true

  depends_on = [
    aws_vpclattice_resource_configuration.node_00,
    aws_vpclattice_resource_configuration.node_01,
    aws_ram_resource_association.mongo,
    aws_ram_principal_association.clickhouse,
  ]
}

resource "clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns" "mongo" {
  service_id                  = var.service_id
  reverse_private_endpoint_id = clickhouse_clickpipes_reverse_private_endpoint.mongo.id

  # DNS name order does not match CHILD order. Match the CHILD ID instead.
  # [0] intentionally fails if no target matches, rather than using a default.
  mapping = [
    {
      private_dns_name  = "node-00-pri.mongo.example.com"
      internal_dns_name = [for n in clickhouse_clickpipes_reverse_private_endpoint.mongo.dns_names : n if strcontains(n, ".${aws_vpclattice_resource_configuration.node_00.id}.")][0]
    },
    {
      private_dns_name  = "node-01-pri.mongo.example.com"
      internal_dns_name = [for n in clickhouse_clickpipes_reverse_private_endpoint.mongo.dns_names : n if strcontains(n, ".${aws_vpclattice_resource_configuration.node_01.id}.")][0]
    },
  ]
}
