You can use the *clickhouse_service_backup_bucket* resource to export the backups of a ClickHouse Cloud service to a bucket in your own cloud account (external backups, also called bring-your-own-bucket). In the ClickHouse Cloud console this is the *External backup* setting in the service's *Backups* section.

This is separate from the `backup_configuration` attribute of *clickhouse_service*, which configures the built-in backups. There can be at most one backup bucket per service.

~> **Note:** This resource is in beta. The API key needs the ADMIN role and the `control-plane:service:manage-backups` permission.

## Providers

The attributes depend on `bucket_provider`. Attributes of other providers must not be set.

| `bucket_provider` | Required attributes |
|---|---|
| `AWS` | `bucket_path`, `iam_role_arn` (optional: `iam_role_session_name`) |
| `GCP` | `bucket_path`, `access_key_id`, `secret_access_key` |
| `AZURE` | `container_name`, `connection_string` |

Changing `bucket_provider` or `service_id` replaces the resource.

## Secrets

`secret_access_key` and `connection_string` are sensitive and are never returned by the API. Terraform therefore cannot detect them being changed outside of Terraform. They are always sent on update.

## Overwrite protection

`Create` performs a `GET` before the `POST` so that an existing bucket surfaces a "please import" diagnostic. The check is best-effort.

## Import

```sh
terraform import clickhouse_service_backup_bucket.example <service_id>
```

Secrets are not imported. After an import, set `secret_access_key` / `connection_string` in your configuration; the next apply updates the bucket in place.
