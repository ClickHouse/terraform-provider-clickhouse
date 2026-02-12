## Organization Settings Example

This example demonstrates how to manage organization-level settings in ClickHouse Cloud.

### Resources Managed:
- `clickhouse_organization_settings` - Configures organization-wide settings (e.g., core dumps)

**Note:** This resource manages settings for your existing organization. It does not create or delete organizations.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
