terraform {
  required_providers {
    clickhouse = {
      version = "0.0.2"
      source  = "ClickHouse/clickhouse"
      # source  = "clickhouse.cloud/terraform/clickhouse" # used for dev
    }
  }
}

variable "organization_id" {
  type = string
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

provider clickhouse {
  environment     = "qa"
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

resource "clickhouse_service" "service" {
  name           = "My Terraform Service"
  cloud_provider = "aws"
  region         = "us-east-1"
  tier           = "production"
  idle_scaling   = true
  password_hash  = "13d249f2cb4127b40cfa757866850278793f814ded3c587fe5889e889a7a9f6c"

  ip_access = [
    {
      source      = "192.168.2.63"
      description = "Test IP"
    }
  ]

  min_total_memory_gb  = 24
  max_total_memory_gb  = 360
  idle_timeout_minutes = 5
}
