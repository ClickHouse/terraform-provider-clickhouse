# This file is generated automatically please do not edit
terraform {
  required_providers {
    clickhouse = {
      version = "3.8.3-alpha2"
      source  = "ClickHouse/clickhouse"
    }
  }
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

provider "google" {
  project     = var.gcp_project_id
  region      = var.gcp_region
}
