#!/usr/bin/env python3
"""Changelog entries from PR descriptions.

Every PR description carries a `## Changelog` section (see
.github/pull_request_template.md). Its text is what ends up in the release
notes. Write `None` there for changes that are not user-facing.

Usage:
  changelog.py check
      Reads the PR description from the PR_BODY env var and fails if the
      Changelog section is missing or empty.

  changelog.py release-notes <tag> [<previous-tag>]
      Collects the Changelog entries of every PR merged between the previous
      tag and <tag>, and prints the release notes as markdown. Without
      <previous-tag>, a stable tag diffs against the previous stable tag and a
      pre-release tag against the previous tag of any kind. Needs `git` with
      full tag history and an authenticated `gh` CLI (GH_REPO env var selects
      the repository).

Bot PRs (e.g. dependabot) are exempt: they are skipped by the release notes and
by the CI check.
"""

import json
import os
import re
import subprocess
import sys

HEADING = re.compile(r"^#{1,6}\s+(.*?)\s*#*\s*$")
COMMENT = re.compile(r"<!--.*?-->", re.DOTALL)
NONE = re.compile(r"^(none|n/a)\.?$", re.IGNORECASE)
BULLET = re.compile(r"^\s*[-*+]\s+")
STABLE_TAG = re.compile(r"^v\d+\.\d+\.\d+$")
PR_NUMBER = re.compile(r"\(#(\d+)\)\s*$")
# Conventional Commit PR title, e.g. "feat(postgres)!: add thing".
TITLE = re.compile(r"^(\w+)(\([^)]*\))?(!)?:")

SECTIONS = [
    ("breaking", "💥 Backward Incompatible Changes"),
    ("feat", "✨ New Features"),
    ("fix", "🐛 Bug Fixes"),
    ("other", "🔧 Other Changes"),
]


def extract(body):
    """Return the Changelog section text, or None if there is no such section."""
    body = COMMENT.sub("", body or "")
    lines = body.splitlines()
    for i, line in enumerate(lines):
        m = HEADING.match(line.strip())
        if m and m.group(1).lower() == "changelog":
            section = []
            for rest in lines[i + 1 :]:
                if HEADING.match(rest.strip()):
                    break
                section.append(rest.rstrip())
            return "\n".join(section).strip()
    return None


def is_none(entry):
    return bool(NONE.match(entry.strip()))


def check():
    entry = extract(os.environ.get("PR_BODY", ""))
    if entry is None:
        print("::error::PR description has no '## Changelog' section. "
              "Add one (see .github/pull_request_template.md), or write 'None' "
              "under it if the change is not user-facing.")
        return 1
    if not entry:
        print("::error::The '## Changelog' section of the PR description is "
              "empty. Describe the user-facing change, or write 'None'.")
        return 1
    if is_none(entry):
        print("Changelog: None (not included in release notes)")
    else:
        print("Changelog entry:\n" + entry)
    return 0


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout


def previous_tag(tag):
    tags = run("git", "tag", "--list", "v*", "--sort=-version:refname").split()
    older = tags[tags.index(tag) + 1 :] if tag in tags else tags
    if STABLE_TAG.match(tag):
        older = [t for t in older if STABLE_TAG.match(t)]
    return older[0] if older else None


def bullets(entry, pr):
    """Format an entry as markdown bullets, each suffixed with the PR link."""
    # Each bullet or paragraph is one item; wrapped lines join the item above.
    items = []
    new_item = True
    for line in entry.splitlines():
        if not line.strip():
            new_item = True
        elif BULLET.match(line) or new_item:
            items.append(BULLET.sub("", line).strip())
            new_item = False
        else:
            items[-1] += " " + line.strip()
    return [f"- {item} (#{pr})" for item in items]


def release_notes(tag, prev=None):
    prev = prev or previous_tag(tag)
    rev = f"{prev}..{tag}" if prev else tag
    subjects = run("git", "log", "--first-parent", "--format=%s", rev).splitlines()
    prs = [int(m.group(1)) for s in subjects if (m := PR_NUMBER.search(s))]

    grouped = {key: [] for key, _ in SECTIONS}
    missing = []
    for pr in reversed(prs):  # oldest first
        data = json.loads(run("gh", "pr", "view", str(pr), "--json", "title,body,author"))
        m = TITLE.match(data["title"])
        kind = "other"
        if m and m.group(3):
            kind = "breaking"
        elif m and m.group(1) in ("feat", "fix"):
            kind = m.group(1)
        entry = extract(data["body"])
        if not entry:
            if data["author"].get("is_bot"):
                continue
            # PRs merged before the Changelog section was required: fall back
            # to the title for user-facing types and flag them for review.
            missing.append(pr)
            if kind == "other":
                continue
            entry = PR_NUMBER.sub("", data["title"][m.end():]).strip()
        if is_none(entry):
            continue
        grouped[kind].extend(bullets(entry, pr))

    out = []
    if not STABLE_TAG.match(tag):
        out.append("> ⚠️ **Beta release** — this version includes features that are still in preview.\n")
    for key, title in SECTIONS:
        if grouped[key]:
            out.append(f"## {title}\n")
            out.extend(grouped[key])
            out.append("")
    if not any(grouped.values()):
        out.append("No user-facing changes in this release.\n")
    if missing:
        out.append("<!-- PRs without a Changelog entry (title used for feat/fix, others skipped), review before publishing: "
                   + ", ".join(f"#{n}" for n in missing) + " -->\n")
    return "\n".join(out).rstrip() + "\n"


def main(argv):
    if len(argv) >= 2 and argv[1] == "check":
        return check()
    if len(argv) in (3, 4) and argv[1] == "release-notes":
        sys.stdout.write(release_notes(*argv[2:]))
        return 0
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
