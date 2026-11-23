You can use the *clickhouse_clickpipe* resource to create and manage ClickPipes data ingestion pipelines in ClickHouse Cloud.

Supported source types: Kafka (Confluent, MSK, Azure Event Hubs, Redpanda, WarpStream), Object Storage (S3, GCS, Azure Blob), Kinesis, Postgres CDC, MySQL CDC, BigQuery, and MongoDB CDC.

Known limitations:

- ClickPipe does not support table updates for managed tables. If you need to update the table schema, you will have to do that externally.
- Changing the source type of an existing ClickPipe will force replacement (destroy and recreate).

Table resync (Postgres, MySQL and MongoDB CDC pipes):

- Changing a table mapping's `resync_version`, e.g. incrementing it, resyncs that table: it is copied again from the source with the mapping's current settings, then replaces its current destination table. Replication of the other tables is paused until the resync completes; track its progress in the ClickHouse Cloud console.
- Changing any other attribute of an existing mapping, except `target_table`, requires changing its `resync_version` in the same apply. `target_table` cannot be changed: remove the mapping and add a new one.
- Removing `resync_version`, or setting it on a new mapping, resyncs nothing.
