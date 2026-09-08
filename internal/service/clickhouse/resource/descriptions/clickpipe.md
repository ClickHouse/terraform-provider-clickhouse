You can use the *clickhouse_clickpipe* resource to create and manage ClickPipes data ingestion pipelines in ClickHouse Cloud.

Supported source types: Kafka (Confluent, MSK, Azure Event Hubs, Redpanda, WarpStream), Object Storage (S3, GCS, Azure Blob), Kinesis, Postgres CDC, MySQL CDC, BigQuery, and MongoDB CDC.

Known limitations:

- ClickPipe does not support table updates for managed tables. If you need to update the table schema, you will have to do that externally.
- Changing the source type of an existing ClickPipe will force replacement (destroy and recreate).

### Kinesis Protobuf schema uploads

Set `source.kinesis.format = "Protobuf"` and supply `source.kinesis.protobuf_schema` using `filebase64()` with a `.proto` or serialized `FileDescriptorSet` file. The maximum encoded size is 1 MiB (a file up to 768 KiB). Other formats do not accept `protobuf_schema`.

This requires Kinesis Protobuf schema upload support to be enabled for your organization. The backend selects the first message declared in the last root file; message selection cannot be overridden through this resource.

The schema is sent only when the ClickPipe is created. Changing, adding, or removing it forces replacement; credential updates do not resend it. Terraform preserves the schema in state because the API does not return it. It is marked sensitive to hide it from normal output, but it is still stored in state, so protect access to your state files.

Import cannot recover the uploaded schema. Adding `protobuf_schema` to an imported ClickPipe forces replacement, even if it matches the schema already stored by the backend.
