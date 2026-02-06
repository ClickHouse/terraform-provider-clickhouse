variable "organization_id" {
  type = string
}

variable "token_key" {
  type      = string
  sensitive = true
}

variable "token_secret" {
  type      = string
  sensitive = true
}

resource "clickhouse_organization" "this" {
  core_dumps_enabled = true 
}
