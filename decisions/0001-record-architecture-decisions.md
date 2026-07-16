# Record architecture decisions

* Status: Accepted
* Date: Thu Jul 16 2026

## Context and Problem Statement

As the provider grows, we make architectural choices (package layout, the
`internal/` over `pkg/` split, the service-group structure, release tooling, and
so on) whose rationale is easy to lose. New contributors then re-litigate
settled questions or unknowingly undo a deliberate trade-off. We need a
lightweight, durable way to capture *why* a decision was made, next to the code
but separate from the public Terraform Registry documentation in `docs/`. We
also want tooling that fits a Go project without adding a Node.js dependency.

## Considered Options

* [btr1975/adr-tool](https://github.com/btr1975/adr-tool) — a Go CLI with a valid
  module path (installable as a `go tool` dependency) and deterministic
  directory-local numbering.
* [adr/adr-log](https://github.com/adr/adr-log) — an npm tool that also generates
  an index but requires Node.js.
* [marouni/adr](https://github.com/marouni/adr) — a Go CLI whose module path does
  not match its repo URL (so it cannot be `go install`ed) and which stores state
  in `~/.adr`.
* A single growing design document that everyone edits.

## Decision Outcome

Chosen option: **btr1975/adr-tool**. We record Architecture Decision Records
under `decisions/`, one numbered Markdown file per decision, created with
`adr-tool` (`make adr` / `go tool adr-tool short-adr`; see
[CONTRIBUTING.md](../CONTRIBUTING.md#raising-an-architecture-decision-record-adr)).

`adr-tool` is pinned as a `go tool` dependency in `go.mod`, so it needs no
separate install and no Node.js. It is purely directory-based (`-p ./decisions`),
derives the next number from existing files (zero-padded, e.g. `0001`), and uses
built-in templates — no per-machine `~/.adr` state.

Records live outside `docs/` because that directory is reserved for the
generated, public Terraform Registry documentation.

## Consequences

* Positive: decisions and their rationale are versioned alongside the code and
  reviewed in the same pull request that implements them.
* Positive: no Node.js dependency and nothing extra to install — `go tool
  adr-tool` builds from the pinned module.
* Positive: numbering is deterministic and repo-local (scanned from `decisions/`).
* Neutral: `adr-tool` does not generate an index, so readers browse `decisions/`
  directly; each record carries its title, status, and date in its header.
* Neutral: earlier decisions are recorded retroactively only as they resurface.
