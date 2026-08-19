You can use the *clickhouse_udf* resource to create and manage User Defined Functions (UDFs) in ClickHouse Cloud.

~> **Note:** This resource is in beta. Its behavior may change in future provider versions.

Point `source_archive_path` at a ZIP of your function source, and set `source_archive_hash` to `filebase64sha256(...)` of that same file so Terraform knows when to publish a new version. On apply, the provider uploads the archive and waits for the build to finish.

### If a build fails

By default, a failed build fails the apply (`fail_on_build_error = true`). Set it to `false` if you'd rather get a warning and move on. Either way, the failed version stays in state, and applying again with the same source won't retry the build — you'll need to fix the source and give `source_archive_hash` a new value first.

On create, a failed build also taints the resource, so the next apply replaces it from scratch. On update, the previous, working version keeps running while you sort things out.

### If Terraform loses track mid-build

Occasionally Terraform can't confirm a publish went through, or polling for the build status times out. Either way, it stops rather than risk creating a duplicate version, and saves the last known status in state. A `terraform plan` (or `refresh`) will pick up the real status, so it's worth doing that before applying again.

### A couple of things to know

`function_name` can't reuse a built-in ClickHouse function name (like `sum`) — the API rejects reserved names.

Deleting this resource removes all versions of the UDF and detaches it from every service that uses it.
