# ClickHouse Terraform Provider

[![Docs](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/docs.yaml/badge.svg)](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/docs.yaml)
[![Dependabot Updates](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/dependabot/dependabot-updates/badge.svg)](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/dependabot/dependabot-updates)
[![Unit tests](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/test.yaml/badge.svg)](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/test.yaml)

This is the official terraform provider for [ClickHouse Cloud](https://clickhouse.com/docs/en/about-us/cloud).

## Usage

You can find examples in the [examples/full](https://github.com/ClickHouse/terraform-provider-clickhouse/tree/main/examples/full) directory.

Please refer to the [official docs](https://registry.terraform.io/providers/ClickHouse/clickhouse/latest/docs) for more details.

## Breaking changes and deprecations

### Upgrading to version >= 1.1.0

In version 1.1.0 we deprecated the `min_total_memory_gb` and `max_total_memory_gb` fields. You can keep using them, but they will eventually be removed.

The intended replacement for those fields are:

- `min_replica_memory_gb`: Minimum memory used by *each replica* during auto-scaling 
- `max_replica_memory_gb`: Maximum memory used by *each replica* during auto-scaling

The key difference between the old and new fields is that the old ones indicated a *total amount of memory* for the whole service (the sum of all replicas) while the new ones act on a *single replica*.

For example, if you had a 3 replica cluster with the following settings:

```terraform
resource "clickhouse_service" "svc" {
  ...
  min_total_memory_gb = 24
  max_total_memory_gb = 36
}
```

you should convert it to

```terraform
resource "clickhouse_service" "svc" {
  ...
  min_replica_memory_gb = 8
  max_replica_memory_gb = 12
}
```

### Upgrading to version >= 1.0.0 of the Clickhouse Terraform Provider

If you are upgrading from version < 1.0.0 to anything >= 1.0.0 and you are using the `clickhouse_private_endpoint_registration` resource or the `private_endpoint_ids` attribute of the `clickhouse_service` resource,
then a manual process is required after the upgrade.

1) In the `clickhouse_private_endpoint_registration` resource, rename the `id` attribute to `private_endpoint_id`.

Before:

```
resource "clickhouse_private_endpoint_registration" "example" {
  id = aws_vpc_endpoint.pl_vpc_foo.id
  ...
}
```

After:

```
resource "clickhouse_private_endpoint_registration" "example" {
  private_endpoint_id = aws_vpc_endpoint.pl_vpc_foo.id
  ...
}
```

2) If you used the `private_endpoint_ids` in any of the `clickhouse_service` resources

For each service with `private_endpoint_ids` attribute set:

2a) Create a new `clickhouse_service_private_endpoints_attachment` resource  like this:

```
resource "clickhouse_service_private_endpoints_attachment" "example" {
  # The ID of the service with the `private_endpoint_ids` set
  service_id = clickhouse_service.aws_red.id

  # the same attribute you previously defined in the `clickhouse_service` resource goes here now
  # Remember to change `id` with `private_endpoint_id` in the `clickhouse_private_endpoint_registration` reference.
  private_endpoint_ids = [clickhouse_private_endpoint_registration.example.private_endpoint_id]
}
```

2b) Remove the `private_endpoint_ids` attribute from the `clickhouse_service` resource.

Example:

Before:

```
resource "clickhouse_service" "example" {
  ...
  private_endpoint_ids = [clickhouse_private_endpoint_registration.example.id]
}
```

After:

```
resource "clickhouse_service" "example" {
  ...
}

resource "clickhouse_service_private_endpoints_attachment" "red_attachment" {
  private_endpoint_ids = [clickhouse_private_endpoint_registration.example.private_endpoint_id]
  service_id = clickhouse_service.example.id
}
```

If everyting is fine, there should be no changes in existing infrastructure but only one or more `clickhouse_service_private_endpoints_attachment` should be pending creation. That is the expected status.

If you have trouble, please open an issue and we'll try to help!

## Development and contributing

Please read the [Development readme](https://github.com/ClickHouse/terraform-provider-clickhouse/blob/main/development/README.md)
