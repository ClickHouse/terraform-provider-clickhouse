resource "clickhouse_table" "mytable" {
  service_id = clickhouse_service.service.id
  database = clickhouse_database.mydatabase.name
  name = "mytable"
  comment = "This is a test table"
  order_by = "id"
  engine = {
    name = "SharedMergeTree"
    params = [
      "'/clickhouse/tables/{uuid}/{shard}'",
      "'{replica}'"
    ]
  }
  settings = {
    index_granularity = 2048
  }
  columns = {
    id = {
      type = "UInt8"
    }
    description = {
      type = "String"
      nullable = true
      comment = "The product description"
    }
    sector = {
      type = "String"
      ephemeral = true
    }
    function_default = {
      type = "String"
      default = "now()"
    }
    literal_default = {
      type = "String"
      default = "'Department 1'"
    }
    mat1 = {
      type = "String"
      materialized = "'Example literal'"
    }
    alias = {
      type = "DateTime"
      alias = "now()"
    }
  }
}
