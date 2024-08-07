terraform {
  required_providers {
    clickhouse = {
      version = "0.0.10"
      source  = "ClickHouse/clickhouse"
    }
  }
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}
