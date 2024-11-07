# This file is generated automatically please do not edit
terraform {
  required_providers {
    clickhouse = {
      version = "1.2.5"
      source  = "ClickHouse/clickhouse"
    }
  }
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}
