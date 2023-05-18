terraform {
  required_providers {
    clickhouse = {
      # version = "0.1"
      source  = "clickhouse.cloud/terraform/clickhouse"
    }
  }
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

provider clickhouse {
  environment     = "local"
  organization_id = "aee076c1-3f83-4637-95b1-ad5a0a825b71"
  token_key       = var.token_key
  token_secret    = var.token_secret
}

resource "clickhouse_service" "service" {
  name           = "My Service"
  cloud_provider = "aws"
  region         = "us-east-1"
  tier           = "production"
  idle_scaling   = true

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
