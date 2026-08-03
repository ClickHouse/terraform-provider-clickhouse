resource "clickhouse_udf" "echo_string" {
  function_name = "echo_string"
  runtime       = "python3.11"
  type          = "executable_pool"
  return_type   = "String"

  arguments = [
    { name = "input", type = "String" },
  ]

  source_archive_path = "${path.module}/echo_string.zip"
  source_archive_hash = filebase64sha256("${path.module}/echo_string.zip")

  pool_size                  = 5
  command_read_timeout       = 10000
  command_write_timeout      = 10000
  max_command_execution_time = 10
}
