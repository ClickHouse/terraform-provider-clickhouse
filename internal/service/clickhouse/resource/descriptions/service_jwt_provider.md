You can use the *clickhouse_service_jwt_provider* resource to let users connect to a ClickHouse Cloud service with JSON Web Tokens (JWT) issued by your own identity provider.

The service verifies each token's signature against the provider's JSON Web Key Set (JWKS) and checks its `iss` and `aud` claims. Users who sign in with a JWT are mapped to the `default` user. If `roles_claim` is set, they also receive the ClickHouse roles listed in that claim.

~> **Note:** This resource is in beta. JWT authentication providers require the Enterprise tier. The API returns `403 FORBIDDEN` if the organization doesn't have the feature or the API key lacks `control-plane:service:manage`.

## Requirements

- `jwks_url` must be a public `https://` URL. The JWKS must contain at least one RS256 (RSA) key. ClickHouse Cloud fetches and validates it when the provider is created or updated, so the apply fails if the URL is unreachable.
- A service can have at most 5 JWT providers.
- `name` must be unique within the service. It can only contain lowercase letters, digits and hyphens, with at most 59 characters. It cannot be `cloud`, `clickhouse`, `ch`, `internal`, `system` or `default`, or start with one of these followed by a hyphen.

Changing `service_id` or `name` replaces the provider. All other attributes are updated in place. Existing providers can be imported with `<service_id>:<name>`.
