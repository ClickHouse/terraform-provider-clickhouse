# Adopt an AWS-style service-group layout with a ServicePackage registry

* Status: Accepted
* Date: Thu Jul 16 2026

## Context and Problem Statement

ClickHouse ships more than one Terraform surface — the Cloud provider, the
separate ClickStack (HyperDX) provider, and potentially DBOps later. Maintaining
these as independent providers duplicates release plumbing, credential handling,
and conventions, and forces users to configure several providers to manage
related resources. We needed a structure that lets multiple product areas live
in one provider without their resources, ownership, or release cadence bleeding
into each other. The full rationale, alternatives, and cross-team discussion are
in [`docs/rfcs/0001-consolidate-providers-and-service-groups.md`](../docs/rfcs/0001-consolidate-providers-and-service-groups.md).

## Decision Outcome

**Single provider.** We consolidate the ClickStack provider (and, in future,
DBOps) into the ClickHouse Cloud provider rather than shipping separate
providers. This was settled with the Terraform/API group in Slack and mirrors how
ClickHouse's internal Terraform tooling is organised: one provider, one set of
credentials plumbing, one release pipeline.

**Service groups.** Resources and data sources are grouped by product area under
`internal/service/<group>/` (AWS-provider style): `clickhouse`, `postgres`, and
`clickstack`. Each group implements a `service.ServicePackage` interface
(`internal/service/service.go`) that self-describes its metadata (name, human
name, CODEOWNERS team, stability) and the resources/data sources it contributes.
A single `internal/service/registry` package lists the groups; the provider
composes its resource and data-source sets from that registry, so adding a group
touches one shared file plus the group's own package — never `provider.go`'s
factory lists.

**`pkg/` → `internal/`.** As part of this restructure all non-`main` code moved
from `pkg/` to `internal/`, so the compiler enforces that nothing here is an
importable public API (see
[`0001`](0001-record-architecture-decisions.md) and
[`GO_CONVENTIONS.md`](../GO_CONVENTIONS.md)).

**Naming: `clickhouse_<component>_<resource>`.** Resources from a non-Cloud group
are prefixed with the component name so they never collide with a Cloud resource
of the same name — e.g. ClickStack's `role` becomes `clickhouse_clickstack_role`,
avoiding a clash with the Cloud `clickhouse_role`. The Cloud group keeps its
existing unprefixed names for state compatibility.

**Deferred: annotation-driven codegen.** We considered generating the registry
and boilerplate from annotations (as the AWS provider does). That is deferred:
the hand-written `ServicePackage` implementations are small and explicit, and the
registry validation test guards against duplicate type names. Codegen can be
revisited once the number of groups or resources makes the boilerplate costly.

## Consequences

* Positive: one provider, one credential surface, one release pipeline; users
  configure a single provider (optionally aliased) for Cloud and ClickStack.
* Positive: groups are isolated — ownership routes via CODEOWNERS per
  `internal/service/<group>/`, and a group can be alpha while others are stable.
* Positive: adding a group is mechanical and the registry test proves no
  Terraform type-name collisions across groups.
* Neutral: existing Cloud resource/data-source type names are unchanged, so
  state stays compatible; only new groups carry the component prefix.
* Neutral: the `resource` / `datasource` subpackages within a group are not yet
  merged into one package per group; group owners can do that incrementally.
