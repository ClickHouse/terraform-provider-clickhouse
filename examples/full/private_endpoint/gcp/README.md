# GCP Private Service Connect example

Tested with HashiCorp/google v6.x Terraform provider.

The Terraform code deploys the following resources:

- 1 ClickHouse service with internet access blocked
- 1 GCP VPC network and subnet
- 1 Private Service Connect (PSC) forwarding rule connecting to the ClickHouse service attachment
- 1 Private DNS zone with a wildcard A record resolving ClickHouse hostnames to the PSC endpoint IP

The ClickHouse service is reachable from within the VPC via the PSC endpoint only; access from the internet is blocked.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all required values.
- Set `gcp_credentials` to the contents of a GCP service account JSON key file (or leave it empty to use Application Default Credentials).
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`

## Required GCP APIs

The following GCP APIs must be enabled in your project:

- Compute Engine API (`compute.googleapis.com`)
- Cloud DNS API (`dns.googleapis.com`)

## Required GCP permissions

The service account used needs the following roles (or equivalent permissions):

- `roles/compute.networkAdmin` — to create VPC networks, subnets, addresses, and forwarding rules
- `roles/dns.admin` — to create private DNS zones and records
