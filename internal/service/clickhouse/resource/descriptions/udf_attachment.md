You can use the *clickhouse_udf_attachment* resource to attach a UDF version to a ClickHouse Cloud service.

~> **Note:** This resource is in alpha. Its behavior may change in future provider versions.

A service can only have one version of a function attached at a time. `version` supports three ways to pick what gets attached:

- **Pin a version:** set `version` to a fixed number. Terraform keeps that version attached until you change it.
- **Track the latest version:** set `version = clickhouse_udf.<name>.version`. Each new UDF version changes this value, so Terraform attaches it on the next apply.
- **Attach once, then leave alone:** omit `version`. Terraform attaches whatever is ready at create time and stores that number in state. Publishing a newer version afterwards does not move the attachment, since there's no config value telling Terraform to.

Only ready versions can be attached. Attaching can take several minutes. If the service is idle, Terraform wakes it first.

If attachment polling times out, Terraform saves the last observed attachment status in state. Refresh or run plan to check whether the attachment completed before applying again.
