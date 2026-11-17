List the custom instance profiles available to the organization in a given
region, e.g. dynamic BYOC profiles like `v1-standard-byoc-4`, or the Gen 2
hardware profile `v2-default`.

`v2-default` is listed only for organizations enabled for Gen 2 on a SCALE
or ENTERPRISE plan (or through a per-organization override), in AWS regions
where Gen 2 is available, and never with `byoc_id`.

Pass `byoc_id` to include the profiles configured for that BYOC infrastructure — BYOC
profiles are only returned when it is set, and `region_id` can then be
omitted (the BYOC infrastructure's region is used; if both are set they must
match). Without `byoc_id`, `region_id` is required. The `profile` value can
be passed to the `profile` attribute of the `clickhouse_service` resource.
Returns an empty list when no custom profiles are available (e.g.
organizations without ENTERPRISE custom profiles, without Gen 2 in that
region, or without BYOC infrastructure).
Read-only.
