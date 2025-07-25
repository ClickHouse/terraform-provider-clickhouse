---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "clickhouse_service_private_endpoints_attachment Resource - clickhouse"
subcategory: ""
description: |-
  Use the clickhouse_service_private_endpoints_attachment resource to attach a ClickHouse service to a Private Endpoint.
  Important: Please note that if you want to attach the same ClickHouse service to multiple Private Endpoints you have to specify all the Private Endpoint IDs in a single clickhouse_service_private_endpoints_attachment resource.
  Having multiple clickhouse_service_private_endpoints_attachment resources for the same service is unsupported and the outcome is unpredictable.
  See private_endpoint_registration https://registry.terraform.io/providers/ClickHouse/clickhouse/latest/docs/resources/private_endpoint_registration for how to create a private endpoint.
  See full example https://github.com/ClickHouse/terraform-provider-clickhouse/tree/main/examples/full/private_endpoint on GitHub.
---

# clickhouse_service_private_endpoints_attachment (Resource)

Use the *clickhouse_service_private_endpoints_attachment* resource to attach a ClickHouse *service* to a *Private Endpoint*.
Important: Please note that if you want to attach the same ClickHouse *service* to multiple *Private Endpoints* you have to specify all the *Private Endpoint IDs* in a single *clickhouse_service_private_endpoints_attachment* resource.
Having multiple *clickhouse_service_private_endpoints_attachment* resources for the same service is unsupported and the outcome is unpredictable.

See [private_endpoint_registration](https://registry.terraform.io/providers/ClickHouse/clickhouse/latest/docs/resources/private_endpoint_registration) for how to create a *private endpoint*.

See [full example](https://github.com/ClickHouse/terraform-provider-clickhouse/tree/main/examples/full/private_endpoint) on GitHub.

## Example Usage

```terraform
resource "clickhouse_service" "svc" {
  ...
}

resource "clickhouse_private_endpoint_registration" "endpoint" {
  ...
}

resource "clickhouse_service_private_endpoints_attachment" "attachment" {
  private_endpoint_ids = [
    clickhouse_private_endpoint_registration.endpoint.id
  ]
  service_id = clickhouse_service.svc.id
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `private_endpoint_ids` (List of String) List of private endpoint IDs
- `service_id` (String) ClickHouse Service ID

## Import

Import is supported using the following syntax:

The [`terraform import` command](https://developer.hashicorp.com/terraform/cli/commands/import) can be used, for example:

```shell
# Endpoint Attachments can be imported by specifying the clickhouse service UUID
terraform import clickhouse_service_private_endpoints_attachment.example xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```
