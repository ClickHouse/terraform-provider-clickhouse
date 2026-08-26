You can use the `clickhouse_clickpipes_ssh_key` resource to create a standalone, service-scoped SSH key resource for ClickPipes.

The server generates an Ed25519 keypair and returns the OpenSSH `public_key`. Install that public key in your bastion's `authorized_keys` so that ClickPipes can tunnel through it. The private key never leaves ClickHouse Cloud.

Once created, reference the resource from a ClickPipe source via its `ssh_key_resource_id` attribute (supported on Kafka, Postgres, MySQL, and MongoDB sources) instead of providing inline SSH configuration. A reference is mutually exclusive with inline SSH config and is immutable on the pipe; changing it forces the pipe to be recreated.

The resource lifecycle is:

- `pending` — created, connectivity not yet validated.
- `active` — the last connectivity validation succeeded.
- `failed` — the last connectivity validation failed.

Connection fields (`host`, `port`, `username`) and `name` are immutable; changing any of them forces resource replacement. Only `description` can be updated in place.

~> **Note:** Connectivity validation requires the public key to already be installed on the bastion, so it is a day-2 operation performed out of band and is not run automatically when the resource is created.
