## AWS deletion-protection example

The Terraform code deploys the following resources:
- 1 ClickHouse service on AWS with deletion protection turned on

> **Note:** This example needs a provider version that supports `deletion_protection`, or a locally built provider (see `development/README.md`). The organization must have service deletion protection enabled, or the apply fails with a 400 error. Creating a service with `deletion_protection = true` needs an API key that can delete all services in the organization.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init` (skip it when you use a locally built provider through `dev_overrides`)
- Run `terraform apply -var-file=variables.tfvars`. `terraform destroy` now fails because the service is protected.
- Run `terraform apply -var-file=variables.tfvars -var update_pass=true` to turn protection off.
- Run `terraform destroy -var-file=variables.tfvars`

`e2e-import.sh` is the e2e check that runs between the two applies: it confirms destroy is refused, the service survives it, and an import shows the protection.
