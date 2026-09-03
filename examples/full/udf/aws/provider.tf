# This file is generated automatically please do not edit
terraform {
  required_providers {
    clickhouse = {
      version = "3.26.0"
      source  = "ClickHouse/clickhouse"
    }
    archive = {
      version = "~> 2.4"
      source  = "hashicorp/archive"
    }
  }
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}
