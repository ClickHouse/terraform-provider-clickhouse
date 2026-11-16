# This file is generated automatically please do not edit
terraform {
  required_providers {
    clickhouse = {
      version = "3.42.0"
      source  = "ClickHouse/clickhouse"
    }
    confluent = {
      source = "confluentinc/confluent"
    }
  }
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

provider "confluent" {
  cloud_api_key    = var.confluent_cloud_api_key
  cloud_api_secret = var.confluent_cloud_api_secret
}
