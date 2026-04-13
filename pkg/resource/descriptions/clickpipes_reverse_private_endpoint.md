You can use the *clickhouse_clickpipes_reverse_private_endpoint* resource to create and manage reverse private endpoints for secure ClickPipes data source connections in ClickHouse Cloud.

Supported endpoint types: `VPC_ENDPOINT_SERVICE`, `VPC_RESOURCE`, and `MSK_MULTI_VPC`.

~> **Note:** All fields on this resource are immutable after creation. Any change will force replacement (destroy and recreate).
