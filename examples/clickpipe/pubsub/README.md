## ClickPipe GCP Pub/Sub

This example demonstrates how to deploy a Pub/Sub ClickPipe using GCP service account authentication.

The `gcp_service_account_b64` variable expects the **base64-encoded** contents of a GCP service account JSON key. To generate it:

```sh
base64 -w 0 < path/to/service-account.json
```

Rotate the key by changing `gcp_service_account_b64` and re-applying.

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
