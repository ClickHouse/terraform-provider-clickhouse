variable "aws_region" {
  type = string
}

provider "aws" {
  region     = var.aws_region
}

variable "vpc_foo_id" {
  type = string
}

variable "vpc_foo_private_link_subnets" {
  type = list(string)
}

variable "vpc_bar_id" {
  type = string
}

variable "vpc_bar_private_link_subnets" {
  type = list(string)
}

// Security group for PrivateLink in VPC foo
resource "aws_security_group" "allow_clickhouse_cloud_foo" {
  name        = "allow_clickhouse_cloud_foo"
  description = "Allow Connections to clickhouse cloud"

  tags = {
    Name = "allow_clickhouse_cloud"
  }
}

// Allow connections from 0.0.0.0/0, please make adjustments
resource "aws_vpc_security_group_ingress_rule" "allow_clickhouse_native_protocol" {
  security_group_id = aws_security_group.allow_clickhouse_cloud_foo.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 9440
  to_port           = 9440
}

// Allow connections from 0.0.0.0/0, please make adjustments
resource "aws_vpc_security_group_ingress_rule" "allow_clickhouse_https_protocol" {
  security_group_id = aws_security_group.allow_clickhouse_cloud_foo.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 8443
  to_port           = 8443
}

// Security group for PrivateLink in VPC bar
resource "aws_security_group" "allow_clickhouse_cloud_bar" {
  vpc_id      = var.vpc_bar_id
  name        = "allow_clickhouse_cloud_bar"
  description = "Allow Connections to clickhouse cloud"

  tags = {
    Name = "allow_clickhouse_cloud"
  }
}

// Allow connections from 0.0.0.0/0, please make adjustments
resource "aws_vpc_security_group_ingress_rule" "allow_clickhouse_native_protocol_bar" {
  security_group_id = aws_security_group.allow_clickhouse_cloud_bar.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 9440
  to_port           = 9440
}

// Allow connections from 0.0.0.0/0, please make adjustments
resource "aws_vpc_security_group_ingress_rule" "allow_clickhouse_https_protocol_bar" {
  security_group_id = aws_security_group.allow_clickhouse_cloud_bar.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 8443
  to_port           = 8443
}

// Private Link in VPC foo
resource "aws_vpc_endpoint" "pl_vpc_foo" {
  vpc_id            = var.vpc_foo_id
  service_name      = data.clickhouse_private_endpoint_config.endpoint_config.endpoint_service_id
  vpc_endpoint_type = "Interface"
  security_group_ids = [
    aws_security_group.allow_clickhouse_cloud_foo.id
  ]
  subnet_ids          = var.vpc_foo_private_link_subnets
  private_dns_enabled = true
}

// Private Link in VPC bar
resource "aws_vpc_endpoint" "pl_vpc_bar" {
  vpc_id            = var.vpc_bar_id
  service_name      = data.clickhouse_private_endpoint_config.endpoint_config.endpoint_service_id
  vpc_endpoint_type = "Interface"
  security_group_ids = [
    aws_security_group.allow_clickhouse_cloud_bar.id
  ]
  subnet_ids          = var.vpc_bar_private_link_subnets
  private_dns_enabled = true
}