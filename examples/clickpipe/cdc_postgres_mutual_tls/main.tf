variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse Cloud service ID"
}

variable "postgres_host" {
  description = "PostgreSQL host address"
}

variable "postgres_port" {
  description = "PostgreSQL port"
  default     = 5432
}

variable "postgres_database" {
  description = "PostgreSQL database name"
}

variable "postgres_username" {
  description = "PostgreSQL username"
  sensitive   = true
}

variable "postgres_password" {
  description = "PostgreSQL password"
  sensitive   = true
}

variable "ca_certificate" {
  description = "PEM encoded CA certificate that signed the PostgreSQL server certificate"
}

variable "client_certificate" {
  description = "PEM encoded client certificate presented to PostgreSQL for mutual TLS"
  sensitive   = true
}

variable "client_private_key" {
  description = "PEM encoded private key for the client certificate"
  sensitive   = true
}

# Postgres CDC ClickPipe authenticating with a client certificate (mutual TLS).
#
# The PostgreSQL server must be configured to request client certificates, e.g.
# with a `hostssl ... cert` or `hostssl ... scram-sha-256 clientcert=verify-full`
# rule in pg_hba.conf.
resource "clickhouse_clickpipe" "postgres_mutual_tls" {
  name       = "Postgres mTLS ClickPipe"
  service_id = var.service_id

  source = {
    postgres = {
      host     = var.postgres_host
      port     = var.postgres_port
      database = var.postgres_database

      credentials = {
        username = var.postgres_username
        password = var.postgres_password
      }

      # Optional: CA certificate to validate the server certificate.
      ca_certificate = var.ca_certificate

      # Client certificate and private key for mutual TLS. Requires TLS, so it
      # cannot be combined with `disable_tls = true`. The values are write-only
      # in the API: change the block to rotate the certificate, remove it to
      # stop presenting a client certificate.
      mutual_tls = {
        certificate = var.client_certificate
        private_key = var.client_private_key
      }

      settings = {
        replication_mode = "cdc"
      }

      table_mappings = [
        {
          source_schema_name = "public"
          source_table       = "users"
          target_table       = "users"
        }
      ]
    }
  }

  destination = {
    database = "default"
  }
}

output "clickpipe_id" {
  value = clickhouse_clickpipe.postgres_mutual_tls.id
}
