# This file is generated automatically please do not edit
terraform {
  required_providers {
    clickhouse = {
      version = "3.1.2"
      source  = "ClickHouse/clickhouse"
    }
  }
}

# Configuration-based authentication
# these keys are for example only and won't work when pointed to a deployed ClickHouse OpenAPI server
provider "clickhouse" {
  organization_id = "aee076c1-3f83-4637-95b1-ad5a0a825b71"
  token_key       = "avhj1U5QCdWAE9CA9"
  token_secret    = "4b1dROiHQEuSXJHlV8zHFd0S7WQj7CGxz5kGJeJnca"
}
