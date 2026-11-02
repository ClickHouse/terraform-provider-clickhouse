# Query-based CDC: each table is polled using a TIMESTAMP watermark column.
resource "clickhouse_clickpipe" "bigquery_cdc_query_based_clickpipe" {
  name = "BigQuery CDC (query based) ClickPipe"

  service_id = "dc189652-b621-4bee-9088-b5b4c3f88626"

  source = {
    bigquery = {
      snapshot_staging_path = "gs://my-staging-bucket/"

      credentials = {
        # Base64-encoded service account JSON key
        service_account_file = "ewogICJuYW1lIjogInByb2plY3RzL1BST0pFQ1RfSUQvc2VydmljZUFjY291bnRzL1NFUlZJQ0VfQUNDT1VOVF9FTUFJTC9rZXlzL0tFWV9JRCIsCiAgInByaXZhdGVLZXlUeXBlIjogIlRZUEVfR09PR0xFX0NSRURFTlRJQUxTX0ZJTEUiLAogICJwcml2YXRlS2V5RGF0YSI6ICJFTkNPREVEX1BSSVZBVEVfS0VZIiwKICAidmFsaWRBZnRlclRpbWUiOiAiREFURSIsCiAgInZhbGlkQmVmb3JlVGltZSI6ICJEQVRFIiwKICAia2V5QWxnb3JpdGhtIjogIktFWV9BTEdfUlNBXzIwNDgiCn0="
      }

      settings = {
        replication_mode                = "cdc"
        replication_method              = "query_based"
        sync_interval_seconds           = 60
        sync_delay_seconds              = 60
        query_cdc_pull_sync_parallelism = 4
      }

      table_mappings = [{
        source_dataset_name        = "test_dataset"
        source_table               = "test_table"
        target_table               = "test_table_cdc"
        query_cdc_watermark_column = "updated_at"
      }]
    }
  }

  destination = {
    database = "default"
  }
}

# Events-based CDC: changes are read from the BigQuery change history.
resource "clickhouse_clickpipe" "bigquery_cdc_events_based_clickpipe" {
  name = "BigQuery CDC (events based) ClickPipe"

  service_id = "dc189652-b621-4bee-9088-b5b4c3f88626"

  source = {
    bigquery = {
      snapshot_staging_path = "gs://my-staging-bucket/"

      credentials = {
        # Base64-encoded service account JSON key
        service_account_file = "ewogICJuYW1lIjogInByb2plY3RzL1BST0pFQ1RfSUQvc2VydmljZUFjY291bnRzL1NFUlZJQ0VfQUNDT1VOVF9FTUFJTC9rZXlzL0tFWV9JRCIsCiAgInByaXZhdGVLZXlUeXBlIjogIlRZUEVfR09PR0xFX0NSRURFTlRJQUxTX0ZJTEUiLAogICJwcml2YXRlS2V5RGF0YSI6ICJFTkNPREVEX1BSSVZBVEVfS0VZIiwKICAidmFsaWRBZnRlclRpbWUiOiAiREFURSIsCiAgInZhbGlkQmVmb3JlVGltZSI6ICJEQVRFIiwKICAia2V5QWxnb3JpdGhtIjogIktFWV9BTEdfUlNBXzIwNDgiCn0="
      }

      settings = {
        # `cdc_only` skips the initial snapshot and only replicates new changes.
        replication_mode   = "cdc_only"
        replication_method = "events_based"
      }

      table_mappings = [{
        source_dataset_name = "test_dataset"
        source_table        = "test_table"
        target_table        = "test_table_events"
        events_function     = "changes"
      }]
    }
  }

  destination = {
    database = "default"
  }
}
