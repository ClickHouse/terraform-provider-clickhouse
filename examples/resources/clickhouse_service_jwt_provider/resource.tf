resource "clickhouse_service" "svc" {
  ...
}

resource "clickhouse_service_jwt_provider" "example" {
  service_id = clickhouse_service.svc.id
  name       = "my-oidc-provider"

  issuer      = "https://auth.example.com"
  audience    = "my-clickhouse-service"
  jwks_url    = "https://auth.example.com/.well-known/jwks.json"
  roles_claim = "clickhouse:roles"
}
