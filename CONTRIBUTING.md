# Contributing

Thanks for your interest in contributing to the ClickHouse Terraform provider.

## Getting started

1. Install [Go](https://go.dev/doc/install) >= 1.26.
2. Fork and clone the repository.
3. Enable the git hooks:

   ```sh
   make enable_git_hooks
   ```

   This symlinks the committed [`.githooks/`](.githooks/) hooks (`pre-commit`
   and `commit-msg`) into `.git/hooks/`. Most tooling is fetched on demand by the
   Make targets: `golangci-lint` and the patched `tfplugindocs` are downloaded
   automatically the first time you run `make fmt` / `make docs`, and
   `adr-tool` / `go-test-coverage` are `go tool` dependencies pinned in
   [`go.mod`](go.mod). Only [`goreleaser`](https://goreleaser.com) must be
   installed separately if you want to run `make goreleaser-check` locally.

## Development workflow

All commands are exposed through the [`Makefile`](Makefile) so that local runs match CI:

| Command                 | Purpose                                                        |
| ----------------------- | -------------------------------------------------------------- |
| `make build`            | Build the provider binary.                                     |
| `make fmt`              | Format Go source (`gofumpt` + `goimports` via golangci-lint).  |
| `make lint`             | Run `go vet` and `golangci-lint`.                              |
| `make sec`              | Run security analysis (`gosec`) on its own.                    |
| `make test`             | Run unit tests.                                                |
| `make testacc`          | Run acceptance tests (creates real resources).                 |
| `make cover`            | Run tests and enforce coverage thresholds (`.testcoverage.yml`). |
| `make docs-alpha`       | Regenerate registry documentation with `tfplugindocs`.         |
| `make docs-check`       | Fail if generated docs are out of date.                        |
| `make goreleaser-check` | Validate both release configs and run a snapshot build.        |
| `make adr`              | Create a new ADR (`title="..." statement="..."`).              |
| `make mock`             | Regenerate the Cloud API client mock.                          |

## Git hooks

`make enable_git_hooks` symlinks the committed [`.githooks/`](.githooks/) hooks
into `.git/hooks/`. Bypass either in an emergency with `git commit --no-verify`.

### `pre-commit`

The [`pre-commit`](.githooks/pre-commit) hook runs `make fmt docs build` and
aborts the commit if any step fails. The remaining checks (`lint`, `sec`,
coverage, docs staleness, goreleaser config) run as separate CI jobs on every
pull request.

### `commit-msg`

The [`commit-msg`](.githooks/commit-msg) hook enforces
[Conventional Commits](#commit-messages) via
[`scripts/check-conventional-commit.sh`](scripts/check-conventional-commit.sh):
it rejects the commit if the subject line does not match
`<type>[optional scope][!]: <description>`. Merge, revert, and `fixup!`/`squash!`
subjects are allowed through unchanged.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/).
Each commit subject must take the form:

```text
<type>[optional scope][!]: <description>
```

- **type** — one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`,
  `build`, `ci`, `chore`, `revert`.
- **scope** *(optional)* — the area of the codebase; prefer the service group
  where applicable, e.g. `feat(clickstack): ...`, `fix(postgres): ...`.
- **`!`** *(optional)* — marks a breaking change, e.g. `fix!: ...`.

Examples:

```text
feat(clickstack): add connection resource
fix!: drop support for Terraform < 1.0
docs: explain the ADR workflow
```

The repository squash-merges pull requests, so the **PR title** becomes the
commit subject on `main` and is validated by the
[`Validate PR title`](.github/workflows/conventional-commits.yaml) CI check. The
`commit-msg` hook enforces the same format locally for every commit. Conventional
Commits keep history machine-readable, which makes changelog generation and
semantic versioning straightforward.

## Raising an Architecture Decision Record (ADR)

Significant architectural choices are recorded as Architecture Decision Records
under [`decisions/`](decisions/) — separate from [`docs/`](docs/), which is
reserved for the generated public Terraform Registry documentation. Raise an ADR
whenever you make a decision that is hard to reverse or that future contributors
would otherwise have to reverse-engineer (package layout, dependencies, release
mechanics, public behavior, and so on).

Records are managed with [`adr-tool`](https://github.com/btr1975/adr-tool), a Go
CLI pinned as a `go tool` dependency in [`go.mod`](go.mod) (no separate install,
no Node.js). It is purely directory-based: numbers are zero-padded and assigned
automatically from the highest existing record, and templates are built in. There
is no generated index — we rely on the [`decisions/`](decisions/) listing and each
record's header.

1. **Create a record:**

   ```sh
   make adr title="Short decision title" statement="The decision and its context"
   ```

   This writes `decisions/000N-short-decision-title.md` from the built-in
   template. (Equivalent: `go tool adr-tool short-adr -p ./decisions -t "..." -s "..."`.)

2. **Fill it in.** Complete the considered options and decision outcome, and set
   the `Status` (`Proposed`, `Accepted`, `Rejected`, `Deprecated`, or
   `Superseded`). The title is the file's single top-level heading.
3. **Commit the record.** Prefer raising the ADR in the same pull request that
   implements the decision, so the rationale is reviewed alongside the change.

For richer records or lifecycle changes, use the tool directly:

```sh
go tool adr-tool long-adr      -p ./decisions -t "Title" -d "Deciders" -s "Statement"
go tool adr-tool change-status -p ./decisions -a 0001-some-adr.md -s accepted
go tool adr-tool supersede     --help
```

> [!TIP]
> `adr-tool`'s `--options/-o` flag splits values on commas, so avoid commas
> inside a single option (use a dash or semicolon instead).

## Adding a resource or data source

1. Implement it in a service group under
   [`internal/service/<group>/`](internal/service/) (`clickhouse`, `postgres`,
   `clickstack`, …).
2. Register it in the group's `ServicePackage` (`Resources()` / `DataSources()`
   in `internal/service/<group>/<group>.go`) — **never** in `provider.go`, which
   composes its resources from the [`registry`](internal/service/registry/).
3. Add an example under [`examples/`](examples/) and run `make docs-alpha`.
4. Add acceptance tests and run `make testacc`.

See [`GO_CONVENTIONS.md`](GO_CONVENTIONS.md) for package layout and naming, and
[`decisions/0002-adopt-service-group-layout.md`](decisions/0002-adopt-service-group-layout.md)
for why the provider is organised into service groups.

## Code conventions

Go code in this repository follows the conventions documented in
[`GO_CONVENTIONS.md`](GO_CONVENTIONS.md) — package layout (`internal/` over
`pkg/`, service groups), `Must*` vs error-returning naming, error handling, and
the rules that `golangci-lint` enforces automatically.

## Releases

> Note: the release process can only be run by ClickHouse employees.

Releases are produced by [GoReleaser](https://goreleaser.com) and published by the
[`Release`](.github/workflows/release.yaml) workflow. The provider version embedded
in the binary is injected at build time via `-X internal/project.version`, and the
published checksums are signed with GPG.

There are two GoReleaser configurations, selected by the shape of the release tag:

| Tag form                                 | Config                   | Used for                                              |
| ---------------------------------------- | ------------------------ | ----------------------------------------------------- |
| `vX.Y.Z` (e.g. `v1.2.3`)                 | `.goreleaser-stable.yml` | Stable releases. Published as a normal GitHub release. |
| `vX.Y.Z-<suffix>` (e.g. `v1.2.3-alpha1`) | `.goreleaser-alpha.yml`  | Pre-release / alpha builds.                            |

The **alpha** config compiles with the `alpha` Go build tag, so any code gated
behind `//go:build alpha` is included, and its GitHub release is never served as
the "latest" version — it must be requested explicitly by version.

Validate the configs locally before releasing with `make goreleaser-check`.

## Pull requests

- Keep changes focused and include tests where practical.
- Ensure `make lint test docs-check` passes before opening a PR.
- Use a Conventional Commit PR title (it becomes the squash commit subject).
- Describe user-facing changes clearly in the PR description.

## Code of conduct

Be respectful and constructive. Report unacceptable behavior to the maintainers.
