You can use the *clickhouse_clickpipe* resource to create and manage ClickPipes data ingestion pipelines in ClickHouse Cloud.

Supported source types: Kafka (Confluent, MSK, GCMK, Azure Event Hubs, Redpanda, WarpStream), Object Storage (S3, GCS, Azure Blob), Kinesis, Postgres CDC, MySQL CDC, BigQuery, and MongoDB CDC.

GCP workload identity authentication is available in Private Preview for GCMK, GCS object storage, and Pub/Sub sources. Set `authentication = "SERVICE_ACCOUNT_WORKLOAD_IDENTITY"` and omit customer credentials. Use the `clickhouse_clickpipes_service_context` data source to retrieve the tenant GCP service account principal, then grant that principal access to the source before creating the ClickPipe.

When the service and ClickPipe are managed in the same configuration, reference the context principal from the customer IAM grant and make the ClickPipe depend on that grant. The context data source waits for the tenant identity by default, producing the dependency chain service -> ready identity -> IAM grant -> ClickPipe. The ClickPipe resource also performs a bounded 30-second readiness check before create or before changing a source to workload identity.

Known limitations:

- ClickPipe does not support table updates for managed tables. If you need to update the table schema, you will have to do that externally.
- Changing the source type of an existing ClickPipe will force replacement (destroy and recreate).
