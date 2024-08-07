terraform {
  required_providers {
    clickhouse = {
      version = "0.0.10"
      source  = "ClickHouse/clickhouse"
    }

    azapi = {
      source  = "Azure/azapi"
      version = "1.13.1"
    }

  }
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}
