# This is the official provider for ClickHouse Cloud.

With this provider you can deploy a ClickHouse instance on AWS, Google Cloud or Azure Cloud.

To use this provider, you need to [Sign In](https://clickhouse.cloud/signIn) for a ClickHouse Cloud account and generate an [API key](https://clickhouse.com/docs/en/cloud/manage/openapi).

You can find more example on how to use this provider on [Github](https://github.com/ClickHouse/terraform-provider-clickhouse/tree/main/examples/full).

Visit [https://clickhouse.com/docs/en/cloud-quick-start](https://clickhouse.com/docs/en/cloud-quick-start) now to get started using ClickHouse Cloud.

## Breaking changes

Note: we only provide upgrade path from consecutive major releases of our terraform provider.
If you are upgrading, please be sure to not skip any major release while you do so.

For example:

- `0.3.0` to `1.0.0` is a valid upgrade path
- `0.3.0` to `1.1.0` is a valid upgrade path
- `0.3.0` to `2.0.0` is NOT a valid upgrade path

### Upgrading to version >= 3.2.0 of the Clickhouse Terraform Provider

In version 3.2.0, we introduced a change to the `Private Endpoints` feature that requires an update on your end. If you are using the `clickhouse_private_endpoint_registration` resource, this affects you. Please visit [https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations](https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations) for more details.

### Upgrading to version >= 3.0.0 of the Clickhouse Terraform Provider

In version 3.0.0 we revisited how to deal with `clickhouse_service` endpoints.
If you are using the `endpoint_config` attribute or the `endpoints` read only attribute, this breaking change affects you.
Please visit [https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations](https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations) for more details.

### Upgrading to version >= 1.0.0 of the Clickhouse Terraform Provider

If you are upgrading from version < 1.0.0 to anything >= 1.0.0 and you are using the `clickhouse_private_endpoint_registration` resource or the `private_endpoint_ids` attribute of the `clickhouse_service` resource,
then a manual process is required after the upgrade. Please visit [https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations](https://github.com/ClickHouse/terraform-provider-clickhouse#breaking-changes-and-deprecations) for more details.

## ClickStack (alpha)

This provider also manages [ClickStack](https://clickhouse.com/docs/use-cases/observability/clickstack) (HyperDX) resources via the `clickhouse_clickstack_*` resources and data sources. These are in **alpha**: they emit an alpha warning at plan/apply time and their behavior may change in future releases.

ClickStack uses its own credentials, separate from the ClickHouse Cloud credentials above:

- `clickstack_api_key` (or the `CLICKSTACK_API_KEY` environment variable) — required to use any `clickhouse_clickstack_*` resource.
- `clickstack_endpoint` (or `CLICKSTACK_ENDPOINT`) — defaults to `https://hyperdx-api.clickhouse.cloud` (ClickStack Cloud); set it to point at a self-hosted ClickStack/HyperDX instance.

Cloud and ClickStack credentials are independent. You can configure only one set: a provider block with just ClickStack credentials is valid (Cloud resources then error if used, and vice versa). To manage both from one configuration, use an aliased provider:

```hcl
provider "clickhouse" { # ClickHouse Cloud
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

provider "clickhouse" { # ClickStack (OSS or Cloud)
  alias              = "clickstack"
  clickstack_api_key = var.clickstack_api_key
  # clickstack_endpoint defaults to https://hyperdx-api.clickhouse.cloud
}
```
