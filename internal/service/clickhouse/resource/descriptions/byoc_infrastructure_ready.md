Wait until a BYOC infrastructure reaches the `infra-ready` state.

A restricted-access BYOC infrastructure (`clickhouse_byoc_infrastructure` with
`access_model = "restricted"`) is created in the `infra-awaiting-connector`
state and only becomes ready once the connector the customer installs has
enrolled and set up the platform layer. Make `clickhouse_service` resources
depend on this resource so they are created only once the infrastructure can
host them.

Creating the resource polls the infrastructure for up to 90 minutes and fails
if it becomes `infra-degraded`, `infra-terminating` or `infra-terminated`
instead. Once created, the resource keeps the recorded readiness: it is not
refreshed, and a later degradation does not plan a new wait. Destroying it
only removes it from the state.

~> **Note:** This resource is in beta. Its behavior may change in future provider versions.
