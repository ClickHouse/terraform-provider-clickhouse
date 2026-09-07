resource "clickhouse_clickpipes_ssh_key" "bastion" {
  service_id  = "3a10a385-ced2-452e-abb8-908c80976a8f"
  name        = "production-bastion"
  description = "Bastion for the prod VPC"
  host        = "bastion.example.com"
  port        = 22
  username    = "clickpipes"
}

# The server generates the keypair; install the returned public key on the
# bastion's authorized_keys so ClickPipes can tunnel through it.
output "bastion_public_key" {
  value = clickhouse_clickpipes_ssh_key.bastion.public_key
}

# Reference the SSH key resource from a ClickPipe source instead of inline SSH config.
resource "clickhouse_clickpipe" "postgres_via_bastion" {
  service_id = "3a10a385-ced2-452e-abb8-908c80976a8f"
  name       = "postgres-cdc"

  source = {
    postgres = {
      host                = "10.0.0.5"
      port                = 5432
      database            = "app"
      ssh_key_resource_id = clickhouse_clickpipes_ssh_key.bastion.id

      # ... remaining Postgres source configuration ...
    }
  }

  # ... destination, scaling, etc. ...
}
