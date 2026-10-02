variable "byoc_id" {
  description = "ID of an existing BYOC infrastructure (e.g. created from the ClickHouse Cloud console)."
  type        = string
}

data "clickhouse_byoc_infrastructure" "this" {
  id = var.byoc_id
}

# Deploy a service into the existing BYOC infrastructure.
resource "clickhouse_service" "byoc" {
  name           = "byoc service"
  cloud_provider = data.clickhouse_byoc_infrastructure.this.cloud_provider
  region         = data.clickhouse_byoc_infrastructure.this.region_id
  byoc_id        = data.clickhouse_byoc_infrastructure.this.id
}

output "byoc_state" {
  value = data.clickhouse_byoc_infrastructure.this.state
}
