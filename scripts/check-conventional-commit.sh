#!/usr/bin/env bash
#
# Validate a commit message against the Conventional Commits 1.0.0 spec
# (https://www.conventionalcommits.org/).
#
# The first non-comment, non-blank line (the subject) must look like:
#
#   <type>[optional scope][optional !]: <description>
#
# e.g.  feat(client): add retry with backoff
#       fix!: drop support for Terraform < 1.0
#       docs: explain the ADR workflow
#
# Usage: check-conventional-commit.sh <path-to-commit-message-file>
#
# Invoked automatically by the committed .githooks/commit-msg hook (enabled with
# `make enable_git_hooks`). Bypass in an emergency with `git commit --no-verify`.

set -euo pipefail

msg_file="${1:?usage: check-conventional-commit.sh <commit-msg-file>}"

# Allowed types per Conventional Commits + the Angular convention superset.
types="feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert"

# First line that is not blank and not a comment is the subject.
subject="$(grep -vE '^[[:space:]]*#' "$msg_file" | grep -vE '^[[:space:]]*$' | head -n1 || true)"

# Let git's own machinery handle special cases: merges, reverts, fixup/squash,
# and amend-with-empty (an empty subject aborts the commit on git's side).
case "$subject" in
	"") exit 0 ;;
	Merge\ *|Revert\ *|fixup!\ *|squash!\ *) exit 0 ;;
esac

# <type>(optional scope)(optional !): <description, at least one char>
pattern="^(${types})(\([a-z0-9._/-]+\))?!?: .+"

if [[ "$subject" =~ $pattern ]]; then
	exit 0
fi

cat >&2 <<EOF
✖ Commit message does not follow Conventional Commits.

  Subject: "$subject"

  Expected: <type>[optional scope][!]: <description>

  Allowed types: ${types//|/, }

  Examples:
    feat(client): add retry with backoff
    fix!: drop support for Terraform < 1.0
    docs: explain the ADR workflow

  See https://www.conventionalcommits.org/ for the full specification.
  Bypass in an emergency with: git commit --no-verify
EOF
exit 1
