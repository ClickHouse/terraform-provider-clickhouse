variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "confluent_cloud_api_key" {
  sensitive = true
}

variable "confluent_cloud_api_secret" {
  sensitive = true
}

variable "service_id" {
  description = "ClickHouse service ID. The service must be in the same AWS region as the Confluent network."
}

variable "region" {
  description = "AWS region of the ClickHouse service and the Confluent network, for example us-east-1."
}

variable "zones" {
  description = "AWS availability zone IDs of the Confluent network, for example [\"use1-az1\", \"use1-az2\", \"use1-az4\"]."
  type        = list(string)
}

variable "clickpipes_aws_account_id" {
  description = "AWS account ID that ClickPipes uses to connect to the Confluent PrivateLink endpoint service."
}

resource "confluent_environment" "clickpipes" {
  display_name = "ClickPipes"
}

resource "confluent_network" "aws" {
  display_name     = "ClickPipes PrivateLink"
  cloud            = "AWS"
  region           = var.region
  connection_types = ["PRIVATELINK"]
  zones            = var.zones

  environment {
    id = confluent_environment.clickpipes.id
  }

  dns_config {
    resolution = "PRIVATE"
  }
}

resource "confluent_private_link_access" "clickpipes" {
  display_name = "ClickPipes"

  aws {
    account = var.clickpipes_aws_account_id
  }

  environment {
    id = confluent_environment.clickpipes.id
  }

  network {
    id = confluent_network.aws.id
  }
}

resource "clickhouse_clickpipes_reverse_private_endpoint" "confluent" {
  service_id                = var.service_id
  description               = "Confluent Cloud PrivateLink"
  type                      = "VPC_ENDPOINT_SERVICE"
  vpc_endpoint_service_name = confluent_network.aws.aws[0].private_link_endpoint_service

  depends_on = [confluent_private_link_access.clickpipes]
}

# Confluent brokers use per-zone hostnames, for example
# b0-<cluster>.use1-az1.<network>.us-east-1.aws.confluent.cloud. Each zonal
# subdomain targets the endpoint DNS name in the same availability zone ID.
# ClickHouse Cloud adds a targeted zone to the endpoint if it is not there yet.
resource "clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns" "confluent" {
  service_id                  = var.service_id
  reverse_private_endpoint_id = clickhouse_clickpipes_reverse_private_endpoint.confluent.id

  mapping = concat(
    [{ private_dns_name = "*.${confluent_network.aws.dns_domain}" }],
    [for az_id, subdomain in confluent_network.aws.zonal_subdomains : {
      private_dns_name = "*.${subdomain}"
      target_id        = az_id
    }],
  )
}

output "reverse_private_endpoint_id" {
  value = clickhouse_clickpipes_reverse_private_endpoint.confluent.id
}

output "dns_targets" {
  value = clickhouse_clickpipes_reverse_private_endpoint.confluent.dns_targets
}
