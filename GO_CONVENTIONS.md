# Go conventions

This document captures the Go conventions for the ClickHouse Terraform provider.
It covers the decisions a linter cannot make for us (package layout, naming,
error handling) and points to the tooling that enforces everything else
automatically.

The guiding principle: **if a rule can be enforced by a tool, enforce it with a
tool.** Only document the conventions that require human judgement.

## Package layout: `internal/` and service groups

All non-`main` code lives under `internal/`. We do not use a top-level `pkg/`
directory for code that is meant to be importable by the outside world, because
**this provider is an application, not a library** — nothing here is a supported
public API.

The reason to reach for `internal/` specifically is that the **Go compiler
enforces privacy** for it. Any package under an `internal/` directory can only be
imported by code rooted at the parent of that `internal/` directory. If anyone
outside the module tries to `import ".../internal/api"`, the build fails. This is
a hard, compiler-level guarantee — not a lint warning, not a naming convention,
not documentation that people ignore.

`pkg/` by contrast is purely conventional. Putting code in `pkg/` signals "this
is public," but the compiler does nothing to stop, scope, or version those
imports. For a provider where every package is an implementation detail, `pkg/`
would advertise a public surface we have no intention of supporting.

**Rule:** new code goes under `internal/`. Reach for `pkg/` only if we ever
deliberately decide to publish a reusable, semver-stable library — and document
that decision when we do.

### Service groups

Resources and data sources are organised into **service groups** under
`internal/service/<group>/` (for example `clickhouse`, `postgres`,
`clickstack`). Each group implements the `service.ServicePackage` interface
(`internal/service/service.go`) and self-describes the resources and data
sources it contributes; the central `internal/service/registry` package is the
one place that lists the groups. See
[`decisions/0002-adopt-service-group-layout.md`](decisions/0002-adopt-service-group-layout.md)
for the rationale.

```
github.com/ClickHouse/terraform-provider-clickhouse/
  main.go                              // wires registry.ServicePackages() into the provider
  internal/
    provider/                          // provider schema + Configure (framework-facing)
    service/
      service.go                       // ServicePackage, Metadata, ProviderData
      registry/                        // the central list of groups
      clickhouse/                      // one service group
        resource/                      // Terraform resources
        datasource/                    // Terraform data sources
      postgres/
      clickstack/
    api/                               // ClickHouse Cloud HTTP/JSON client (no Terraform types)
```

### Layer boundaries

Split packages by responsibility so the compiler can keep layers honest:

- `internal/service/<group>/...` — Terraform schema, resources, and data sources
  (the framework-facing layer).
- `internal/api` and any per-group `client` package (e.g.
  `internal/service/clickstack/client`) — the API client and domain logic,
  **free of any `terraform-plugin-framework` types**.

Keep the client layer ignorant of Terraform. A resource translates between
Terraform models and client types; the client only speaks HTTP/JSON. This keeps
the client independently testable and lets a group be reused or split without
dragging the framework along.

## Naming

Idiomatic Go naming is mostly mechanical and is checked by `staticcheck` /
`govet`. The conventions worth stating explicitly:

### `Must*` for panic-on-failure helpers

A function prefixed with `Must` **panics** instead of returning an error. Use it
only where a failure is a programmer error that should abort startup — package
initialisation, tests, or hard-coded inputs that are known-good at compile time.
This mirrors the standard library (`regexp.MustCompile`, `template.Must`).

```go
// MustParseURL panics if raw is not a valid URL. Use only with compile-time
// constant inputs (e.g. package-level defaults).
func MustParseURL(raw string) *url.URL {
    u, err := url.Parse(raw)
    if err != nil {
        panic(fmt.Sprintf("MustParseURL(%q): %v", raw, err))
    }
    return u
}
```

**Never** call a `Must*` helper on runtime input (user config, API responses,
environment variables). Those paths must return an `error` and surface it as a
Terraform diagnostic.

### The default: return an `error`, do not panic

Go does not have exceptions, and there is **no `Should` prefix** in idiomatic
Go. The default, unprefixed function is the one that *can fail at runtime* and
therefore returns an `error` (or appends to `resp.Diagnostics`). In other words:

| Failure mode                                  | Naming                          |
| --------------------------------------------- | ------------------------------- |
| Cannot fail / programmer error → panic        | `Must<Verb>` (e.g. `MustParse`) |
| Can fail at runtime → return `error`          | `<Verb>` (e.g. `Parse`, `Get`)  |

If we want an explicit marker for "best-effort, ignores failure," prefer
`Try<Verb>` returning `(T, bool)` over inventing a `Should` prefix — but reach
for that only when it genuinely reads better than a plain `error` return.

### Constructors

Use the `New<Type>` form, matching the framework's existing factories
(`NewServiceResource`, `NewRoleDataSource`). A resource/data source file exposes
exactly one `New<Name>Resource` / `New<Name>DataSource` that returns the
framework interface, and the group's `ServicePackage` implementation lists it in
`Resources()` / `DataSources()`.

### Initialisms

Keep acronyms uppercase: `APIKey`, `HTTPClient`, `URL`, `ID`, `JSON` — not
`ApiKey` / `Url` / `Id`. `staticcheck` (ST1003) flags violations.

### Receivers

Short, consistent receiver names per type (`r *ServiceResource`,
`d *roleDataSource`). Don't mix `self`/`this`; don't rename the receiver between
methods of the same type.

## Error handling

- Wrap errors with context using `%w` so callers can `errors.Is` / `errors.As`:
  `fmt.Errorf("create service: %w", err)`.
- In the framework layer, do not return raw `error`s to Terraform — append to
  `resp.Diagnostics` and **return early** on `resp.Diagnostics.HasError()`.
- Every returned `error` must be checked. `errcheck` enforces this; do not
  silently `_ =` an error without a comment explaining why it's safe.

## Context and logging

- Accept `context.Context` as the first parameter on any function that does I/O,
  and thread it through to HTTP calls.
- Log through `tflog` (structured), not `fmt.Println` / `log`. Never log secrets
  (API keys, tokens); schema fields holding them must be marked `Sensitive: true`.

## Interface assertions

Assert interface satisfaction at compile time next to the type:

```go
var _ resource.Resource = (*ServiceResource)(nil)
```

This produces a clear compile error at the definition site instead of a confusing
one at the registration site.

## Testing

- Unit tests are `*_test.go` next to the code; acceptance tests are gated by
  `TF_ACC` and run via `make testacc`.
- Prefer table-driven tests with subtests (`t.Run`).
- Test fixtures may repeat literals and embed fake credentials; the linter is
  already configured to skip `goconst` / `gosec` / `dupl` on `_test.go`.

## What the tooling enforces (don't re-document these)

Formatting, import ordering, and a large class of correctness rules are enforced
by `golangci-lint` (see [`.golangci.yml`](.golangci.yml)) and run in the
pre-commit hook and CI. `gosec` additionally runs on its own via `make sec` —
as a dedicated `security` job in CI — so a hard-coded credential fails fast,
independent of the slower build/lint/docs steps. Do not police these by hand or
restate them as prose rules:

| Concern                                   | Enforced by   |
| ----------------------------------------- | ------------- |
| Formatting (stricter than `gofmt`)        | `gofumpt`     |
| Import grouping + local-prefix ordering   | `goimports`   |
| Unchecked errors                          | `errcheck`    |
| Suspicious constructs / vet rules         | `govet`       |
| Static analysis, naming, deprecations     | `staticcheck` |
| Ineffectual assignments                   | `ineffassign` |
| Loop variable capture bugs                | `copyloopvar` |
| Security issues (e.g. hard-coded creds)   | `gosec`       |
| Unnecessary type conversions              | `unconvert`   |
| Repeated magic strings (client code)      | `goconst`     |
| Copy-pasted code blocks                   | `dupl`        |

`goconst` is scoped off the `resource` / `datasource` packages, where Terraform
schema attribute keys are idiomatic repeated string literals; it stays active on
the client/logic code (`internal/api`, per-group `client`), where magic-string
detection has value.

Import ordering specifically: standard library, then third-party, then this
module's packages (prefix `github.com/ClickHouse/terraform-provider-clickhouse`),
each group separated by a blank line. `goimports` rewrites this automatically —
run `make fmt`.

## Summary

1. New code under `internal/`; rely on the compiler to enforce privacy.
2. Resources/data sources live in a service group under
   `internal/service/<group>/`; register them in the group's `ServicePackage`,
   never in `provider.go`.
3. Keep the client layer free of Terraform types.
4. `Must*` panics (compile-time-safe inputs only); everything else returns an
   `error` / appends diagnostics.
5. `New<Type>` constructors, uppercase initialisms, short receivers.
6. Wrap errors with `%w`; surface failures as diagnostics and return early.
7. Let `golangci-lint` own formatting, imports, and mechanical correctness —
   run `make fmt lint` before committing.
