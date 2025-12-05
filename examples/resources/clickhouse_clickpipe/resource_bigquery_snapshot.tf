resource "clickhouse_clickpipe" "bigquery_snapshot_clickpipe" {
  name = "BigQuery Snapshot ClickPipe"

  service_id = "dc189652-b621-4bee-9088-b5b4c3f88626"

  source = {
    bigquery = {
      snapshot_staging_path = "gs://my-staging-bucket/"

      credentials = {
        # Base64-encoded service account JSON key
        service_account_file = "ewogICJuYW1lIjogInByb2plY3RzL1BST0pFQ1RfSUQvc2VydmljZUFjY291bnRzL1NFUlZJQ0VfQUNDT1VOVF9FTUFJTC9rZXlzL0tFWV9JRCIsCiAgInByaXZhdGVLZXlUeXBlIjogIlRZUEVfR09PR0xFX0NSRURFTlRJQUxTX0ZJTEUiLAogICJwcml2YXRlS2V5RGF0YSI6ICJFTkNPREVEX1BSSVZBVEVfS0VZIiwKICAidmFsaWRBZnRlclRpbWUiOiAiREFURSIsCiAgInZhbGlkQmVmb3JlVGltZSI6ICJEQVRFIiwKICAia2V5QWxnb3JpdGhtIjogIktFWV9BTEdfUlNBXzIwNDgiCn0="
      }

      settings = {
        replication_mode = "snapshot"
      }

      table_mappings = [{
        source_dataset_name = "test_dataset"
        source_table        = "test_table"
        target_table        = "test_table_snapshot"
      }]
    }
  }

  destination = {
    database = "default"
  }
}
