Mint an enrollment token for a restricted-access BYOC infrastructure
(`clickhouse_byoc_infrastructure` with `access_model = "restricted"`). The
connector installed in the customer cluster redeems the token once to enroll
the cluster into ClickHouse Cloud.

Right after the infrastructure is registered, ClickHouse Cloud admits the trust
of the registered cluster, which takes a few minutes. Until then the API
refuses to mint tokens; the provider retries for up to 15 minutes.

The API returns a token only once. Refreshing the resource never mints a new
token, and the token is not renewed when it expires. To mint a new one, for
example to reinstall the connector, replace the resource with
`terraform apply -replace=<resource address>`. After `terraform import`, the
token is null.

The token is stored in the Terraform state; protect the state accordingly.
Destroying the resource only removes it from the state: tokens cannot be
revoked and expire on their own.

~> **Note:** This resource is in beta. Its behavior may change in future provider versions.
