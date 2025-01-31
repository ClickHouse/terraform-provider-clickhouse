data "clickhouse_api_key_id" "my_api_key" {
  # The name attribute can be omitted
  # In this case the API Key used to run the terraform provider will be retrieved.
  name = "my-api-key"
}
