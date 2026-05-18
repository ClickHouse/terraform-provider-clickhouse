## AWS scheduled-scaling example

The Terraform code deploys the following resources:
- 1 ClickHouse service on AWS
- 1 scheduled scaling configuration with two weekly windows (business hours / overnight)

> **Note:** Scheduled scaling is a beta feature. The organization must have the `canUseScheduledAutoscaling` feature enabled.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
