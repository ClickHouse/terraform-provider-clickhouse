You can use the *clickhouse_clickpipe* resource to create and manage ClickPipes data ingestion pipelines in ClickHouse Cloud.

Supported source types: Kafka (Confluent, MSK, Azure Event Hubs, Redpanda, WarpStream), Object Storage (S3, GCS, Azure Blob), Kinesis, Postgres CDC, MySQL CDC, BigQuery, and MongoDB CDC.

Known limitations:

- ClickPipe does not support table updates for managed tables. If you need to update the table schema, you will have to do that externally.
- Changing the source type of an existing ClickPipe will force replacement (destroy and recreate).
