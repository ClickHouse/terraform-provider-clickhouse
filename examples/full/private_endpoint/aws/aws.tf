variable "aws_key" {
  type = string
}

variable "aws_secret" {
  type = string
}

variable "aws_session_token" {
  type = string
  default = ""
}

locals {
  tags = {
    Name = var.service_name
  }
}

provider "aws" {
  region     = var.region
  access_key = var.aws_key
  secret_key = var.aws_secret
  token      = var.aws_session_token
}

resource "aws_vpc" "vpc" {
  cidr_block = "192.168.0.0/16"

  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = local.tags
}

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_subnet" "subnet1" {
  vpc_id     = aws_vpc.vpc.id
  cidr_block = "192.168.0.0/24"
  availability_zone = data.aws_availability_zones.available.names[0]

  tags = local.tags
}

resource "aws_subnet" "subnet2" {
  vpc_id     = aws_vpc.vpc.id
  cidr_block = "192.168.1.0/24"
  availability_zone = data.aws_availability_zones.available.names[1]

  tags = local.tags
}

// Security group for PrivateLink in VPC foo
resource "aws_security_group" "allow_clickhouse_cloud_foo" {
  name        = "allow_clickhouse_cloud_foo"
  description = "Allow Connections to clickhouse cloud"
  vpc_id = aws_vpc.vpc.id

  tags = local.tags
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

// Private Link in VPC foo
resource "aws_vpc_endpoint" "pl_vpc_foo" {
  vpc_id            = aws_vpc.vpc.id
  service_name      = data.clickhouse_private_endpoint_config.endpoint_config.endpoint_service_id
  vpc_endpoint_type = "Interface"
  security_group_ids = [
    aws_security_group.allow_clickhouse_cloud_foo.id
  ]
  subnet_ids          = [
    aws_subnet.subnet1.id,
    aws_subnet.subnet2.id
  ]
  private_dns_enabled = false

  tags = local.tags
}
