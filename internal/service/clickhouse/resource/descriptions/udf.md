You can use the *clickhouse_udf* resource to create and manage User Defined Functions (UDFs) in ClickHouse Cloud.

~> **Note:** This resource is in alpha. Its behavior may change in future provider versions.

Point `source_archive_path` at a ZIP of your function source, and set `source_archive_hash` with `filebase64sha256(...)` so Terraform notices when the zip changes. Apply uploads the archive and waits for the build to finish.

If Terraform cannot confirm that a publish completed, it stops instead of risking a duplicate version. Refresh the UDF and verify its latest version and settings before trying again.

`function_name` cannot reuse a built-in ClickHouse function name (for example, `sum`); the API rejects reserved names.

If a build fails, Terraform keeps the failed version in state. Fix the source, update `source_archive_hash`, and apply again. Applying with the same source will not retry the build.

By default, a failed build fails the apply (`fail_on_build_error = true`). Set it to `false` to get a warning instead. Changing this setting alone does not create a new version.

When build polling times out, Terraform saves the last observed status in state. Refresh or run plan to check the status before applying again.

On create, a failed build also marks the resource as tainted so the next apply replaces it. On later updates, the previous version keeps running on your services.

Deleting this resource removes all versions and detaches the UDF from every service.
