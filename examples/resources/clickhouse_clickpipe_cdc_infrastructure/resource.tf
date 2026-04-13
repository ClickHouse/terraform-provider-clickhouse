resource "clickhouse_clickpipe_cdc_infrastructure" "example" {
  service_id           = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  replica_cpu_millicores = 2000
  replica_memory_gb      = 8
}
