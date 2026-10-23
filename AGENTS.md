# Agent instructions

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and
[GO_CONVENTIONS.md](GO_CONVENTIONS.md) for code style.

## Pull requests

- PR titles must be Conventional Commits (`feat(postgres): ...`, `fix: ...`);
  the repo squash-merges, so the title becomes the commit subject on `main`.
- Every PR description must have a `## Changelog` section (see
  [.github/pull_request_template.md](.github/pull_request_template.md)). CI
  fails without it.
  - Its text is copied verbatim into the release notes, so write it for
    provider users: what they can now do or what was broken and is fixed.
    Name the resource/attribute, skip implementation details.
  - One line, or several `- ` bullets for multiple changes.
  - Mention it if the resource is in beta, or if the change is backward
    incompatible (also mark the title with `!`, e.g. `feat!: ...`).
  - Write `None` for changes that are not user-facing (CI, tests, refactors,
    internal docs).
