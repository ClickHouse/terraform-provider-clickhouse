# ClickHouse terraform provider examples

This directory contains some examples to use the official ClickHouse terraform provider.

- `basic`: contains examples on how to create a basic ClickHouse service in all supported cloud providers.
- `private_endpoint`: contains examples on how to create a ClickHouse service and connect to supported cloud providers using a private link.

## CI region configuration

The E2E, import, and release workflows read region lists from repository
[Actions variables](https://github.com/ClickHouse/terraform-provider-clickhouse/settings/variables/actions):

| API environment | Variable |
| --- | --- |
| Production (including scheduled runs) | `EXAMPLE_REGIONS_PRODUCTION` |
| Staging | `EXAMPLE_REGIONS_STAGING` |
| Development | `EXAMPLE_REGIONS_DEVELOPMENT` |

Each variable contains JSON with `regions` and `compliance_regions` maps, keyed
by cloud provider. For example:

```json
{
  "regions": {
    "aws": ["us-east-1"],
    "azure": ["eastus2"],
    "gcp": ["us-east1"]
  },
  "compliance_regions": {
    "aws": ["us-east-1"],
    "azure": ["eastus2"],
    "gcp": ["us-east1"]
  }
}
```

These values illustrate the format; preserve the approved region lists for each
environment when migrating. The `regions` list for the selected cloud must contain
at least one region. The `compliance_regions` list can be empty in environments
where compliance examples are skipped, such as development. CI randomly chooses
a region from each non-empty list. To exclude `us-central1` in production,
remove it from both GCP arrays in
`EXAMPLE_REGIONS_PRODUCTION`, keeping the other approved regions.

The API URL, organization ID, and API keys continue to come exclusively from the
`API_ENV_PRODUCTION`, `API_ENV_STAGING`, and `API_ENV_DEVELOPMENT` secrets. Do not
copy those credentials into an Actions variable. Custom runs continue to use
their explicit API and region inputs.

### Migrating existing lists

1. Run the **Export example regions** workflow for the desired environment. It
   extracts only `regions` and `compliance_regions` from the existing secret and
   writes them to the job summary; access to the original Bitwarden JSON is not
   required.
2. Copy that JSON into the corresponding repository Actions variable. Edit the
   region arrays as needed, then save the variable.
3. Run the E2E workflow for that environment to verify the selected regions.

During migration, an unset or empty variable falls back to the region lists in
the corresponding secret. Once set, the variable supplies both maps in full;
missing or malformed lists, and an empty `regions` list, fail before any
Terraform work, without falling back to a region that was intentionally removed.
Credentials-only cleanup steps do not require region variables.
