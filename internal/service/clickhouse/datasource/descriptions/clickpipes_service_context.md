Use the `clickhouse_clickpipes_service_context` data source to retrieve service-level ClickPipes capabilities and the GCP service account used for workload identity authentication.

~> **Note:** This data source is in beta. GCP workload identity authentication is in Private Preview.

By default, the data source requires workload identity support, then waits up to 30 seconds for it to be ready and for its principal to become available. This prevents downstream IAM grants and ClickPipes from observing a temporary empty principal immediately after service creation. An unsupported service fails immediately in this mode. Set `wait_for_identity = false` to return the current API snapshot without waiting, including an unsupported or not-yet-ready context.

Grant `gcp_workload_identity.principal` access to the customer source resources before creating the ClickPipe.
