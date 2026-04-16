resource "clickhouse_clickpipe" "mongodb_cdc_clickpipe" {
  name       = "My MongoDB CDC ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source {
    mongodb {
      uri             = "mongodb+srv://cluster0.example.mongodb.net"
      read_preference = "secondaryPreferred"

      credentials {
        username = "mongo_user"
        password = "***"
      }

      settings {
        replication_mode = "cdc"

        # Optional: Sync interval for polling changes (seconds)
        sync_interval_seconds = 30

        # Optional: Number of rows to pull per batch
        pull_batch_size = 500

        # Optional: Number of rows per partition during snapshot
        snapshot_num_rows_per_partition = 100000

        # Optional: Number of collections to snapshot in parallel
        snapshot_number_of_parallel_tables = 2

        # Optional: Enable hard delete behavior in ReplacingMergeTree
        # delete_on_merge = true

        # Optional: Store JSON values in native ClickHouse JSON format
        # use_json_native_format = true
      }

      table_mappings {
        source_database_name = "mydb"
        source_collection    = "users"
        target_table         = "mydb_users"

        # Optional: Specify table engine (default: ReplacingMergeTree)
        # table_engine = "ReplacingMergeTree"
      }

      table_mappings {
        source_database_name = "mydb"
        source_collection    = "orders"
        target_table         = "mydb_orders"
      }
    }
  }

  destination {
    database = "default"

    # Note: For MongoDB CDC, tables are automatically created
    # based on the table_mappings configuration.
    # The destination.table and destination.columns fields are not used.
  }
}
