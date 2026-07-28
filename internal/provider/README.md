# ClickHouse Cloud Terraform Provider

This is the official Terraform provider for [ClickHouse Cloud](https://clickhouse.com/docs/en/about-us/cloud). The provider allows you to safely and predictably manage ClickHouse Cloud resources in a declarative configuration language (i.e., "Infrastructure-as-Code").

The following service groups can be managed using the ClickHouse Cloud Terraform provider:

| Service group | What you can manage |
|---|---|
| **ClickHouse Cloud** | Cloud services and their lifecycle (e.g., auto-scaling, scheduled scaling, upgrade windows), SQL console access control (e.g., organization members, custom roles, API keys), **[ClickPipes](https://clickhouse.com/docs/integrations/clickpipes)**, and other resources. |
| **ClickStack** | [ClickStack](https://clickhouse.com/docs/use-cases/observability/clickstack) (HyperDX) observability resources (e.g., connections, sources, dashboards, alerts, saved searches, teams, roles, webhooks). |
| **Postgres** | [Managed Postgres](https://clickhouse.com/docs/cloud/postgres) services and their lifecycle. |

> This provider allows managing **SQL console-level** access control (i.e., organization roles and permissions). To manage **database-level** access control (i.e., database users, roles, grants), use the separate [`clickhousedbops`](https://github.com/ClickHouse/terraform-provider-clickhousedbops) provider.

For examples on how to use this provider, see the [Github repository](https://github.com/ClickHouse/terraform-provider-clickhouse/tree/main/examples/).

## Authentication

To use this provider, you need a ClickHouse Cloud account. Once you have [signed up for an account](https://console.clickhouse.cloud/signUp), you can [sign in](https://clickhouse.cloud/signIn) and generate an [API key](https://clickhouse.com/docs/en/cloud/manage/openapi) for authentication.

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

How the `clickhouse_clickstack_*` resources authenticate depends on where ClickStack runs:

**ClickStack on ClickHouse Cloud** is served through the [ClickHouse Cloud API](https://clickhouse.com/docs/use-cases/observability/clickstack/api-reference) and authenticates with the same Cloud credentials as the rest of the provider (`organization_id`, `token_key`, `token_secret`). Set `clickstack_service_id` (or the `CLICKSTACK_SERVICE_ID` environment variable) to the ID of the Cloud service running ClickStack:

```hcl
provider "clickhouse" {
  organization_id       = var.organization_id
  token_key             = var.token_key
  token_secret          = var.token_secret
  clickstack_service_id = var.clickstack_service_id
}
```

On ClickHouse Cloud, ClickStack manages connections, sources, dashboards, alerts, saved searches and webhooks. Connections are read-only — the platform provisions them, so an imported connection can be read but not updated or destroyed (use `terraform state rm` to detach one). Roles, teams and team membership are managed through ClickHouse Cloud, not ClickStack — use the `clickhouse_role` and `clickhouse_role_assignment` resources — so the `clickhouse_clickstack_role`, `clickhouse_clickstack_team` and `clickhouse_clickstack_team_member` resources (and the `clickstack_role` data source) are for self-hosted ClickStack only. The `team` attribute on other resources is likewise not applicable on Cloud — a service is a single ClickStack team — and is rejected. Capability checks are server-side: an endpoint the Cloud API does not serve returns a route-not-found error, and newly exposed endpoints work without a provider upgrade.

**Self-hosted ClickStack** (open source or EE) authenticates with its own credentials, separate from the ClickHouse Cloud credentials above:

- `clickstack_api_key` (or the `CLICKSTACK_API_KEY` environment variable) — a personal API access key from the HyperDX UI.
- `clickstack_endpoint` (or `CLICKSTACK_ENDPOINT`) — the API base URL of the deployment, e.g. `http://localhost:8000`. Required together with `clickstack_api_key`.

Cloud and self-hosted ClickStack credentials are independent. You can configure only one set: a provider block with just self-hosted ClickStack credentials is valid (Cloud resources then error if used, and vice versa). To manage both from one configuration, use an aliased provider:

```hcl
provider "clickhouse" { # ClickHouse Cloud (optionally including managed ClickStack)
  organization_id = var.organization_id
  token_key       = var.token_key
  token_secret    = var.token_secret
}

provider "clickhouse" { # self-hosted ClickStack
  alias               = "clickstack"
  clickstack_endpoint = var.clickstack_endpoint
  clickstack_api_key  = var.clickstack_api_key
}
```
