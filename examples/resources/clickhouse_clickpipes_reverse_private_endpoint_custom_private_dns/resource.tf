resource "clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns" "example" {
  service_id                  = "3a10a385-ced2-452e-abb8-908c80976a8f"
  reverse_private_endpoint_id = "12345678-1234-1234-1234-123456789012"

  mapping = [
    {
      private_dns_name = "my-service.example.com"
    }
  ]
}
