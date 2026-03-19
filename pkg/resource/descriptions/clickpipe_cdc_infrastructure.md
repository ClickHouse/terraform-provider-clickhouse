You can use the *clickhouse_clickpipe_cdc_infrastructure* resource to manage scaling settings for CDC ClickPipes infrastructure shared across all DB ClickPipes in a service.

~> **Important:** Only one CDC infrastructure resource per service is supported. Creating multiple instances for the same service will cause conflicts.

This endpoint becomes available once at least one DB ClickPipe has been provisioned. The resource will poll for up to 10 minutes waiting for the endpoint to become available.

For billing purposes, 2 CPU cores and 8 GB of RAM correspond to one compute unit.

~> **Note:** CDC infrastructure is shared across all DB ClickPipes and cannot be explicitly deleted. Removing this resource only removes it from Terraform state. The infrastructure is automatically cleaned up when all DB ClickPipes are deleted.
