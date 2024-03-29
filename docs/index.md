---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "clickhouse Provider"
subcategory: ""
description: |-
  
---

# clickhouse Provider



## Example Usage

```terraform
# Configuration-based authentication
# these keys are for example only and won't work when pointed to a deployed ClickHouse OpenAPI server
provider "clickhouse" {
  organization_id = "aee076c1-3f83-4637-95b1-ad5a0a825b71"
  token_key       = "avhj1U5QCdWAE9CA9"
  token_secret    = "4b1dROiHQEuSXJHlV8zHFd0S7WQj7CGxz5kGJeJnca"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `api_url` (String) API URL of the ClickHouse OpenAPI the provider will interact with. Alternatively, can be configured using the `CLICKHOUSE_API_URL` environment variable. Only specify if you have a specific deployment of the ClickHouse OpenAPI you want to run against.
- `organization_id` (String) ID of the organization the provider will create services under. Alternatively, can be configured using the `CLICKHOUSE_ORG_ID` environment variable.
- `token_key` (String) Token key of the key/secret pair. Used to authenticate with OpenAPI. Alternatively, can be configured using the `CLICKHOUSE_TOKEN_KEY` environment variable.
- `token_secret` (String, Sensitive) Token secret of the key/secret pair. Used to authenticate with OpenAPI. Alternatively, can be configured using the `CLICKHOUSE_TOKEN_SECRET` environment variable.
