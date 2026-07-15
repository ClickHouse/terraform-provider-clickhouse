# Architecture Decision Records

This directory holds the **Architecture Decision Records (ADRs)** for the
ClickHouse Terraform provider — short documents that capture significant
architectural decisions, the context behind them, and their consequences.

> [!NOTE]
> These records are for *contributors*. They are intentionally kept out of
> [`docs/`](../docs/), which is reserved for the generated public Terraform
> Registry documentation.

Records are created with [btr1975/adr-tool](https://github.com/btr1975/adr-tool),
a Go CLI pinned as a `go tool` dependency in [`go.mod`](../go.mod) — so there is
nothing extra to install and no Node.js. There is no generated index: browse the
numbered `*.md` files in this directory; each one carries its title, status, and
date in its header. Numbers are zero-padded and assigned automatically from the
highest existing record (e.g. `0001`, `0002`, …).

## Adding a decision

See the [contributing guide](../CONTRIBUTING.md#raising-an-architecture-decision-record-adr)
for the full workflow. In short:

```sh
make adr title="Short decision title" statement="The decision and its context"
```

This writes `decisions/000N-short-decision-title.md` from the built-in template.
Then fill in the considered options and decision outcome, set the status, and
commit the record.

For long records, status changes, or supersedes, call the tool directly:

```sh
go tool adr-tool --help
go tool adr-tool long-adr     -p ./decisions -t "Title" -d "Deciders" -s "Statement"
go tool adr-tool change-status -p ./decisions -a 0001-some-adr.md -s accepted
go tool adr-tool supersede    --help
```

> [!TIP]
> `--options/-o` values are split on commas, so avoid commas inside an option
> (use a dash or semicolon instead).
