# ClickStack on ClickHouse Cloud E2E Test

Exercises the `clickhouse_clickstack_*` resources against a long-lived ClickHouse
Cloud service with managed ClickStack: `apply` creates a log source, saved
search, webhook, alert and dashboard on it; `refresh` reads them back; `destroy`
removes them. The service itself is not managed here.

## Why the service is long-lived

Two reasons, both of which rule out creating it per run:

- ClickStack has to be onboarded onto the service, which is a console step the
  provider can't do.
- `clickstack_service_id` configures the provider, and provider configuration
  happens before any resource is applied, so a service created in the same apply
  isn't known yet. `depends_on` can't reorder that.

## One-time setup

Once per org, by hand:

1. Create a small Cloud service named **`[e2e]-clickstack-nightly`** via the
   console. CI looks the service up by that exact name
   (`.github/scripts/discover_clickstack_ids.sh`), and the name carries no
   per-run token, so the cleanup job doesn't match it.
2. Seed the schema and get some telemetry in, by running the ClickStack
   collector against the service. It creates the otel tables itself, so don't
   hand-roll them — the source below maps its schema, and a hand-made table
   drifts from it:

   ```bash
   docker run -e CLICKHOUSE_ENDPOINT="https://<service-host>:8443" \
     -e CLICKHOUSE_USER="default" \
     -e CLICKHOUSE_PASSWORD="..." \
     -p 4317:4317 -p 4318:4318 \
     clickhouse/clickstack-otel-collector:latest
   ```

   Then send it anything over OTLP, e.g.:

   ```bash
   docker run --rm --network container:<collector> \
     ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:latest \
     logs --otlp-endpoint localhost:4317 --otlp-insecure --duration 8s
   ```

   Onboarding needs data present: ClickStack creates its default sources once
   telemetry arrives, and the connection id discovery reads comes off one of
   those. `POST /dashboards/validate` also needs a schema to validate against.

3. Finish onboarding managed ClickStack in the console.

## Not covered

Connections. On Cloud the connections endpoint is not exposed: the platform
creates a single connection per service, pointing at itself, so
`clickhouse_clickstack_connection` is self-hosted only.

`clickhouse_clickstack_team` and `clickhouse_clickstack_team_member`: the
ClickStack team endpoints 404 on Cloud by design (see below). Covering them needs
a self-hosted (Docker) harness, not this one.

Roles *are* covered. ClickStack RBAC is a separate system from ClickHouse Cloud's
own RBAC — it governs ClickStack objects (dashboards, saved searches, sources,
webhooks, alerts, notebooks) and a role created through it does not show up in
Cloud's role list. `clickhouse_role` manages Cloud organization RBAC and grants
nothing in ClickStack, so it is not a substitute.

There is no team id to fetch or pass: on Cloud a service *is* the ClickStack
team, so the client never sends the `x-hdx-team` header and errors if a `team`
attribute is set (`internal/service/clickstack/client/client.go`).

## Variables

| Variable | Required | Description |
|---|---|---|
| `organization_id` | Yes | ClickHouse Cloud organization ID |
| `token_key` | Yes | ClickHouse Cloud API key |
| `token_secret` | Yes | ClickHouse Cloud API secret |
| `clickstack_service_id` | Yes | Service running managed ClickStack |
| `connection_id` | Yes | That service's platform-created connection |
| `suffix` | No | Appended to the ClickStack object names |

Every run applies to the same service, so `suffix` is what keeps concurrent runs
apart. CI sets it to the generated service name.

## Running locally

Both ids come from the service. CI discovers them; for a local run, read them off
any source the onboarding created:

```bash
curl -s --user "$TOKEN_KEY:$TOKEN_SECRET" \
  "https://api.clickhouse.cloud/v1/organizations/$ORG_ID/services/$SERVICE_ID/clickstack/sources" \
  | jq -r '.result[0].connection'
```

```bash
cat > variables.tfvars <<EOF
organization_id       = "..."
token_key             = "..."
token_secret          = "..."
clickstack_service_id = "..."
connection_id         = "..."
suffix                = "-local"
EOF

terraform init
terraform apply -var-file=variables.tfvars
terraform destroy -var-file=variables.tfvars
```

## Cleanup

`terraform destroy` removes the ClickStack objects but not the service. A run
that fails before destroy leaves its objects behind on the shared service —
they carry the run's `suffix`, so they're identifiable, but nothing sweeps them
automatically. The `cleanup-clickhouse` job only deletes services.
