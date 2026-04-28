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
  default = "tf-pg-e2e"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "suffix" {
  type    = string
  default = ""
}

resource "clickhouse_postgres_instance" "test" {
  name             = "${var.service_name}${var.suffix}"
  cloud_provider   = "aws"
  region           = var.region
  postgres_version = "17"
  size             = "m6gd.medium"
  storage_size     = 118
  ha_type          = "none"

  pg_config = {
    wal_level            = "logical"
    hot_standby_feedback = "on"
  }

  tags = {
    test       = "true"
    managed_by = "terraform"
  }
}

data "clickhouse_postgres_instance" "lookup" {
  id = clickhouse_postgres_instance.test.id
}

data "clickhouse_postgres_instance_ca_certificate" "cert" {
  postgres_instance_id = clickhouse_postgres_instance.test.id
}

output "instance_id" {
  value = clickhouse_postgres_instance.test.id
}

output "hostname" {
  value = clickhouse_postgres_instance.test.hostname
}

output "state" {
  value = clickhouse_postgres_instance.test.state
}

output "data_source_hostname" {
  value = data.clickhouse_postgres_instance.lookup.hostname
}

output "ca_cert_pem" {
  value     = data.clickhouse_postgres_instance_ca_certificate.cert.pem
  sensitive = true
}
