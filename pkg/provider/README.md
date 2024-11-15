# This is the official provider for ClickHouse Cloud.

With this provider you can deploy a ClickHouse instance on AWS, Google Cloud or Azure Cloud.

To use this provider, you need to [Sign In](https://clickhouse.cloud/signIn) for a ClickHouse Cloud account and generate an [API key](https://clickhouse.com/docs/en/cloud/manage/openapi).

You can find more example on how to use this provider on [Github](https://github.com/ClickHouse/terraform-provider-clickhouse/tree/main/examples/full).

Visit [https://clickhouse.com/docs/en/cloud-quick-start](https://clickhouse.com/docs/en/cloud-quick-start) now to get started using ClickHouse Cloud.

## Breaking changes

### Upgrading to version >= 1.0.0 of the Clickhouse Terraform Provider

If you are upgrading from version < 1.0.0 to anything >= 1.0.0 and you are using the `clickhouse_private_endpoint_registration` resource or the `private_endpoint_ids` attribute of the `clickhouse_service` resource,
then a manual process is required after the upgrade. Please visit [https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations](https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations) for more details.
