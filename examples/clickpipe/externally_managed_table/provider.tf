# This file is generated automatically please do not edit
terraform {
  required_providers {
    clickhouse = {
      version = "2.0.0-alpha1"
      source  = "ClickHouse/clickhouse"
    }
  }
}

variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}
variable "api_url" {
  default = "https://api.clickhouse.cloud/v1"
}

provider "clickhouse" {
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
  api_url = var.api_url
}
