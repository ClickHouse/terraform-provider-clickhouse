data "clickhouse_clickpipes_service_context" "example" {
  service_id = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
}

output "clickpipes_gcp_workload_identity_principal" {
  value = data.clickhouse_clickpipes_service_context.example.gcp_workload_identity.principal
}
