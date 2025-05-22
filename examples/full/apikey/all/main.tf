variable "organization_id" {
  type = string
}

variable "token_key" {
  type = string
}

variable "token_secret" {
  type = string
}

variable "service_name" {
  type = string
  default = "My Terraform Service"
}

resource "clickhouse_api_key" "admin" {
  name  = "ciccio" # var.service_name
  roles = [ "admin" ]
  expiration_date = "2026-01-31 00:00"
}

output "credentials" {
  value = clickhouse_api_key.admin.name
}
