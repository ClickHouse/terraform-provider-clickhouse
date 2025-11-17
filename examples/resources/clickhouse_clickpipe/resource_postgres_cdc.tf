resource "clickhouse_clickpipe" "postgres_cdc_clickpipe" {
  name       = "My Postgres CDC ClickPipe"
  service_id = "e9465b4b-f7e5-4937-8e21-8d508b02843d"

  source {
    postgres {
      host     = "postgres.example.com"
      port     = 5432
      database = "mydb"

      credentials {
        username = "postgres_user"
        password = "***"
      }

      settings {
        replication_mode = "cdc"

        # Optional: Sync interval for polling changes (seconds)
        sync_interval_seconds = 60

        # Optional: Number of rows to pull per batch
        pull_batch_size = 100000

        # Optional: Allow nullable columns in destination tables
        allow_nullable_columns = true

        # Optional: Number of parallel workers for initial snapshot load
        initial_load_parallelism = 4

        # Optional: Number of rows per partition during snapshot
        snapshot_num_rows_per_partition = 100000

        # Optional: Number of tables to snapshot in parallel
        snapshot_number_of_parallel_tables = 1

        # Optional: Publication name (system-managed if not specified)
        # publication_name = "my_publication"

        # Optional: Replication slot name (system-managed if not specified)
        # replication_slot_name = "my_replication_slot"

        # Optional: Enable failover slots for high availability
        # enable_failover_slots = true
      }

      table_mappings {
        source_schema_name = "public"
        source_table       = "users"
        target_table       = "public_users"

        # Optional: Columns to exclude from replication
        # excluded_columns = ["password_hash", "internal_notes"]

        # Optional: Use custom sorting key
        # use_custom_sorting_key = true
        # sorting_keys = ["id", "created_at"]

        # Optional: Specify table engine (default: ReplacingMergeTree)
        # table_engine = "ReplacingMergeTree"
      }

      table_mappings {
        source_schema_name = "public"
        source_table       = "orders"
        target_table       = "public_orders"
      }
    }
  }

  destination {
    database = "default"

    # Note: For Postgres CDC, table and columns are automatically created
    # based on the source schema and table_mappings configuration.
    # The destination.table and destination.columns fields are not used.
  }
}
