# ClickHouse Terraform Provider

[![Docs](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/docs.yaml/badge.svg)](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/docs.yaml)
[![Dependabot Updates](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/dependabot/dependabot-updates/badge.svg)](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/dependabot/dependabot-updates)
[![Unit tests](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/test.yaml/badge.svg)](https://github.com/ClickHouse/terraform-provider-clickhouse/actions/workflows/test.yaml)

This is the official terraform provider for [ClickHouse Cloud](https://clickhouse.com/docs/en/about-us/cloud).

## Usage

You can find examples in the [examples/full](https://github.com/ClickHouse/terraform-provider-clickhouse/tree/main/examples/full) directory.

Please refer to the [official docs](https://registry.terraform.io/providers/ClickHouse/clickhouse/latest/docs) for more details.

## Breaking changes and deprecations

### Upgrading to version >= 3.2.0

In version 3.2.0 we introduced a change in the `Private Endpoints` feature that requires a change on your side if you use this setting.

Before 3.2.0, this was the way to connect a ClickHouse Cloud service running using Private Link to an external VPC:

```
resource "clickhouse_service" "svc1" {
  ...
}

data "clickhouse_private_endpoint_config" "endpoint_config" {
  cloud_provider = "aws"
  region         = var.region
}

resource "aws_vpc_endpoint" "pl_vpc_foo" {
  vpc_id            = aws_vpc.vpc.id
  service_name      = data.clickhouse_private_endpoint_config.endpoint_config.endpoint_service_id
  ...
}

resource "clickhouse_private_endpoint_registration" "private_endpoint_aws_foo" {
  cloud_provider      = "aws"
  private_endpoint_id = aws_vpc_endpoint.pl_vpc_foo.id
  region              = var.region
  description         = "Private Link from VPC foo"
}

resource "clickhouse_service_private_endpoints_attachment" "red_attachment" {
  private_endpoint_ids = [clickhouse_private_endpoint_registration.private_endpoint_aws_foo.private_endpoint_id]
  service_id = clickhouse_service.svc1.id
}
```

After 3.2.0 this became much simpler:

```
resource "clickhouse_service" "svc1" {
  ...
}

resource "aws_vpc_endpoint" "pl_vpc_foo" {
  vpc_id            = aws_vpc.vpc.id
  service_name      = clickhouse_service.svc1.endpoint_config.endpoint_service_id
  ...
}

resource "clickhouse_service_private_endpoints_attachment" "red_attachment" {
  private_endpoint_ids = [aws_vpc_endpoint.pl_vpc_foo.id]
  service_id = clickhouse_service.aws_red.id
}
```

So after upgradring the terraform provider version from < 3.2.0 to >= 3.2.0, please do the following:

- Remove any stanzas of to the `clickhouse_private_endpoint_config` data source
- Remove any stanzas of the `clickhouse_private_endpoint_registration` resource (delete operation is a no-op so you can safely apply)
- Replace any reference to the `clickhouse_private_endpoint_config` data source with the `endpoint_config` attribute of the `clickhouse_service`
- Change the `private_endpoint_ids` value of `clickhouse_service_private_endpoints_attachment` stanza to use `private_endpoint_id` of the `aws_vpc_endpoint` resource

### Upgrading to version >= 3.0.0

In version 3.0.0 we revisited how to deal with `clickhouse_service` endpoints.

If you are using the `clickhouse_service.endpoints_configuration attribute` or reading the `clickhouse_service.endpoints` read only attribute, then you might be affected.

This is a list of all the changes:

- the `endpoints_configuration` attribute was removed. Please use the `endpoints` attribute in a similar fashion. For example if you had

```
resource "clickhouse_service" "service" {
  ...
  endpoints_configuration = {
    mysql = {
      enabled = true
    }
  }
  ...
}
```

you need to replace it with

```
resource "clickhouse_service" "service" {
...
  endpoints = {
    mysql = {
      enabled = true
    }
  }
...
}
```

- the `endpoints` attribute's type changes from a list to a map.

Where before you had:

```
endpoints = [
  {
    protocol = "https"
    host = "ql5ek38hzz.us-east-2.aws.clickhouse.cloud"
    port: 8443
  },
  {
    protocol: "mysql"
    host: "ql5ek38hzz.us-east-2.aws.clickhouse.cloud",
    port: 3306
  },
  ...
]
```

Now you'll have:

```
endpoints = {
  "https": {
    "host": "ql5ek38hzz.us-east-2.aws.clickhouse.cloud",
    "port": 8443
  },
  "mysql": {
    "enabled": false,
    "host": null,
    "port": null
  },
  "nativesecure": {
    "host": "ql5ek38hzz.us-east-2.aws.clickhouse.cloud",
    "port": 9440
  }
}
```

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
