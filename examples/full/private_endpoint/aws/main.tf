variable "organization_id" {
  type = string
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

variable "service_name" {
  type = string
  default = "red"
}

variable "region" {
  type = string
  default = "us-east-2"
}

resource "clickhouse_service" "aws_red" {
  name                 = var.service_name
  cloud_provider       = "aws"
  region               = var.region
  idle_scaling         = true
  idle_timeout_minutes = 5
  password_hash        = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  // keep it empty to block access from internet
  ip_access = []

  backup_configuration = {
    backup_period_in_hours           = 24
    backup_retention_period_in_hours = 24
    backup_start_time                = null
  }

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120
}

resource "clickhouse_service_private_endpoints_attachment" "red_attachment" {
  private_endpoint_ids = [aws_vpc_endpoint.pl_vpc_foo.id]
  service_id = clickhouse_service.aws_red.id
}

# hostname for connecting to instance via PrivateLink from VPC foo
output "red_private_link_endpoint" {
  value = clickhouse_service.aws_red.private_endpoint_config.private_dns_hostname
}
