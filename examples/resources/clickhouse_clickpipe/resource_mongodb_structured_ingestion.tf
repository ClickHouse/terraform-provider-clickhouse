resource "clickhouse_clickpipe" "mongodb_structured_ingestion" {
  name       = "My MongoDB structured ingestion ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source = {
    mongodb = {
      uri             = "mongodb+srv://cluster0.example.mongodb.net"
      read_preference = "secondaryPreferred"

      credentials = {
        username = "mongo_user"
        password = "***"
      }

      settings = {
        replication_mode = "cdc"
      }

      table_mappings = [
        {
          # Inferred schema: columns are omitted, so their types are inferred
          # from a sample of the collection documents when the mapping is created,
          # and the state records the inferred columns that are not excluded.
          # Not available when the pipe uses ssh_key_resource_id.
          source_database_name = "blade_runner"
          source_collection    = "replicants"
          target_table         = "structured_replicants"

          structured_ingestion = {
            # Optional: leave inferred columns out of replication
            excluded_columns = ["internal_notes"]
          }
        },
        {
          # Provided schema: columns are taken as given. Required when the pipe
          # uses ssh_key_resource_id. The _id key is always replicated as String.
          source_database_name = "blade_runner"
          source_collection    = "missions"
          target_table         = "structured_missions"
          table_engine         = "ReplacingMergeTree"

          structured_ingestion = {
            columns = [
              { name = "inception_date", type = "DateTime64(9)" },
              { name = "model", type = "String" },
              { name = "name", type = "String" },
            ]

            # Optional settings, also available with inferred columns.
            # To skip a field, leave it out of columns: excluded_columns
            # only applies to inferred columns.
            sorting_keys           = ["name"]
            partition_by_expr      = "toYYYYMM(inception_date)"
            drop_unexpected_values = false
          }
        },
      ]
    }
  }

  destination = {
    database = "default"
  }
}
