## GCP HIPAA compliant service example

The Terraform code deploys following resources:
- 1 ClickHouse HIPAA compliant service on GCP

The ClickHouse HIPAA compliant service is available from anywhere.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`


## Important note

HIPAA compliance should be enabled for your ClickHouse organization.
