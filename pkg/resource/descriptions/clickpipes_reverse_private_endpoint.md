You can use the *clickhouse_clickpipes_reverse_private_endpoint* resource to create and manage reverse private endpoints for secure ClickPipes data source connections in ClickHouse Cloud.

Supported endpoint types: `VPC_ENDPOINT_SERVICE`, `VPC_RESOURCE`, `MSK_MULTI_VPC`, and `GCP_PSC_SERVICE_ATTACHMENT`.

~> **Note:** All fields on this resource are immutable after creation. Any change will force replacement (destroy and recreate).
