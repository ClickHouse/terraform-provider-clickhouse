List the custom instance profiles available to the organization in a given
region, e.g. dynamic BYOC profiles like `v1-standard-byoc-4`. Pass `byoc_id`
to include the profiles configured for that BYOC infrastructure — BYOC
profiles are only returned when it is set. The `profile` value can be passed
to the `profile` attribute of the `clickhouse_service` resource. Returns an
empty list when no custom profiles are available (e.g. non-ENTERPRISE,
non-BYOC organization tiers). Read-only.
