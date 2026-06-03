## AWS upgrade-window example

The Terraform code deploys the following resources:
- 1 ClickHouse service on AWS
- 1 upgrade window pinned to Wednesday at 12:00 UTC

> **Note:** Scheduled upgrades is a beta feature. The organization must have the `canUseScheduledUpgrades` feature enabled, and the upgrade window can only be set on primary services.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
